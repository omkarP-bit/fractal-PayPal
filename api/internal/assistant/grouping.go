package assistant

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"fractal/api/internal/risk"
)

// Incident is a merchant-scoped cluster of cases that share a verified root
// cause.
type Incident struct {
	ID                 string    `json:"id"`
	Number             string    `json:"number"`
	MerchantID         string    `json:"merchant_id"`
	ProviderAccountID  string    `json:"provider_account_id"`
	Fingerprint        string    `json:"-"`
	RootCauseCode      string    `json:"root_cause_code"`
	RootCauseSummary   string    `json:"root_cause_summary"`
	AISummary          string    `json:"ai_summary"`
	NextStep           string    `json:"next_step"`
	Status             string    `json:"status"`
	Severity           string    `json:"severity"`
	Confidence         float64   `json:"confidence"`
	PriorityScore      int       `json:"priority_score"`
	CaseCount          int       `json:"case_count"`
	OpenCaseCount      int       `json:"open_case_count"`
	AffectedAmount     int64     `json:"affected_amount_minor"`
	Currency           string    `json:"currency"`
	FirstSeenAt        time.Time `json:"first_seen_at"`
	LastSeenAt         time.Time `json:"last_seen_at"`
	ResolvedAt         *time.Time `json:"resolved_at,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Cases              []IncidentCase `json:"cases"`
	DraftType          string    `json:"draft_type"`
	DraftsPending      int       `json:"drafts_pending"`
	DraftsApproved     int       `json:"drafts_approved"`
	DraftsSent         int       `json:"drafts_sent"`
}

// IncidentCase is a case inside an incident with its triage facts.
type IncidentCase struct {
	CaseID          string    `json:"case_id"`
	CaseNumber      string    `json:"case_number"`
	Status          string    `json:"status"`
	CaseType        string    `json:"case_type"`
	CustomerName    string    `json:"customer_name"`
	CustomerEmail   string    `json:"customer_email"`
	MerchantOrderRe string    `json:"merchant_order_ref"`
	ProviderStatus  string    `json:"provider_status"`
	OrderStatus     string    `json:"order_status"`
	AmountMinor     int64     `json:"amount_minor"`
	Currency        string    `json:"currency"`
	Severity        string    `json:"severity"`
	PriorityScore   int       `json:"priority_score"`
	AgeMinutes      int       `json:"age_minutes"`
	Summary         string    `json:"summary"`
	NextStep        string    `json:"next_step"`
	CreatedAt       time.Time `json:"created_at"`
}

// RootCauseSummaries maps a root-cause code to a merchant-facing sentence.
// The wording is derived from verified facts only.
func RootCauseSummaries() map[string]string {
	return map[string]string{
		RootCauseOrderUpdateMissing: "PayPal confirmed the payment but the store order was never updated, so the webhook delivery path is the likely cause.",
		RootCauseAmountMismatch:     "The captured amount does not match the amount on the store order.",
		RootCauseCurrencyMismatch:   "The captured currency does not match the currency of the store order.",
		RootCauseDuplicateCharge:    "The provider recorded more than one capture against this order.",
		RootCausePaymentUnconfirmed: "The provider has not confirmed a captured payment for this order yet.",
		RootCauseOther:              "The provider and store states do not agree and the specific cause is still being verified.",
	}
}

// PriorityInput holds the verified facts that drive the triage ranking.
type PriorityInput struct {
	Severity     string
	AmountMinor  int64
	Age          time.Duration
	IncidentSize int
}

// ScorePriority computes the triage priority from verified facts only.
// The AI may phrase the summary, but it never sets the priority.
func ScorePriority(in PriorityInput) int {
	severity := 0
	switch strings.ToUpper(in.Severity) {
	case "CRITICAL":
		severity = 100
	case "HIGH":
		severity = 60
	case "MEDIUM":
		severity = 30
	case "LOW":
		severity = 10
	}

	amount := 0
	if in.AmountMinor > 0 {
		amount = int(math.Min(20, math.Log10(float64(in.AmountMinor)/100+1)*10))
	}

	age := int(math.Min(30, in.Age.Hours()))

	size := 0
	if in.IncidentSize > 1 {
		size = int(math.Min(20, float64(in.IncidentSize-1)*5))
	}

	return severity + amount + age + size
}

// SeverityFor derives the operational severity of a case from its verified
// facts using the shared risk engine.
func SeverityFor(f CaseFacts, now time.Time) string {
	evidenceAge := 0
	if !f.OrderFetchedAt.IsZero() {
		evidenceAge = int(now.Sub(f.OrderFetchedAt).Seconds())
	}
	if evidenceAge < 0 {
		evidenceAge = 0
	}
	out := risk.Derive(risk.Input{
		ProviderStatus:     f.ProviderStatus,
		OrderStatus:        f.OrderStatus,
		AmountMismatch:     f.AmountMismatch(),
		CurrencyMismatch:   f.CurrencyMismatch(),
		EvidenceAgeSeconds: evidenceAge,
	})
	return out.Severity
}

// GroupCase clusters one case into an incident.
//
// The AI proposes the grouping through the verified fingerprint; the Go
// backend re-checks the shared facts before the case is accepted. Cases that
// fail verification stay standalone and are still visible in the triage inbox.
func (s *Service) GroupCase(ctx context.Context, caseID string) (*Incident, error) {
	loader := NewFactsLoader(s.db)
	facts, err := loader.LoadOrErr(ctx, caseID)
	if err != nil {
		return nil, err
	}
	if err := facts.VerifySharedFacts(); err != nil {
		// The case does not show the shared discrepancy, so it must not join
		// an incident. It stays visible as an individual triage row.
		return nil, nil
	}

	fingerprint := facts.Fingerprint()
	now := time.Now().UTC()

	incidentID, err := s.findIncident(ctx, facts, fingerprint, now)
	if err != nil {
		return nil, err
	}
	if incidentID == "" {
		incidentID, err = s.createIncident(ctx, facts, fingerprint, now)
		if err != nil {
			return nil, err
		}
		if err := s.linkCase(ctx, incidentID, facts, "AI", 0.6); err != nil {
			return nil, err
		}
	} else {
		confidence := s.linkConfidence(ctx, incidentID, facts)
		if err := s.linkCase(ctx, incidentID, facts, "AI", confidence); err != nil {
			return nil, err
		}
	}

	if err := s.refreshIncident(ctx, incidentID); err != nil {
		return nil, err
	}
	return s.GetIncident(ctx, facts.MerchantID, incidentID)
}

// findIncident looks for an open incident with the same fingerprint whose
// window still covers the case.
func (s *Service) findIncident(ctx context.Context, facts CaseFacts, fingerprint string, now time.Time) (string, error) {
	windowStart := facts.CreatedAt.Add(-s.cfg.Window)
	var incidentID string
	err := s.db.QueryRow(ctx, `
		SELECT id::text FROM incidents
		WHERE merchant_id = $1
		  AND fingerprint = $2
		  AND status IN ('OPEN','INVESTIGATING')
		  AND last_seen_at >= $3
		ORDER BY last_seen_at DESC
		LIMIT 1`, facts.MerchantID, fingerprint, windowStart).Scan(&incidentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return incidentID, nil
}

func (s *Service) createIncident(ctx context.Context, facts CaseFacts, fingerprint string, now time.Time) (string, error) {
	summaries := RootCauseSummaries()
	rootCause := facts.RootCause()
	summary := summaries[rootCause]
	severity := SeverityFor(facts, now)

	var incidentID string
	err := s.db.QueryRow(ctx, `
		INSERT INTO incidents
			(merchant_id, provider_account_id, fingerprint, root_cause_code, root_cause_summary,
			 ai_summary, next_step, status, severity, confidence, first_seen_at, last_seen_at, draft_type)
		VALUES ($1, NULLIF($2, '')::uuid, $3, $4, $5, $6, $7, 'OPEN', $8, $9, $10, $10, 'STATUS_UPDATE')
		RETURNING id::text`,
		facts.MerchantID, facts.ProviderAccountID, fingerprint, rootCause, summary,
		incidentSummary(rootCause, summary, 1), nextStepFor(rootCause), severity,
		0.6, facts.CreatedAt).Scan(&incidentID)
	if err != nil {
		return "", err
	}
	return incidentID, nil
}

// linkConfidence estimates how well a case matches the incident it joins,
// from the number of already-verified sibling cases.
func (s *Service) linkConfidence(ctx context.Context, incidentID string, facts CaseFacts) float64 {
	var siblings int
	err := s.db.QueryRow(ctx, `
		SELECT count(*) FROM incident_cases WHERE incident_id = $1`, incidentID).Scan(&siblings)
	if err != nil {
		return 0.6
	}
	confidence := 0.6 + math.Min(0.3, float64(siblings)*0.1)
	if len(facts.EvidenceIDs) > 0 {
		confidence += 0.05
	}
	if confidence > 0.95 {
		confidence = 0.95
	}
	return confidence
}

func (s *Service) linkCase(ctx context.Context, incidentID string, facts CaseFacts, addedBy string, confidence float64) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO incident_cases (incident_id, case_id, added_by, confidence)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (incident_id, case_id) DO UPDATE
			SET confidence = GREATEST(incident_cases.confidence, EXCLUDED.confidence)`,
		incidentID, facts.CaseID, addedBy, confidence)
	return err
}

// refreshIncident recomputes the incident aggregates from its linked cases.
func (s *Service) refreshIncident(ctx context.Context, incidentID string) error {
	incident, err := s.loadIncidentRow(ctx, incidentID)
	if err != nil {
		return err
	}
	cases, err := s.loadIncidentCases(ctx, incidentID)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	var (
		openCount   int
		amount      int64
		currency    string
		severity    = "LOW"
		oldest      time.Time
		newest      = incident.FirstSeenAt
	)

	for _, c := range cases {
		// The exposure of an incident is the money behind every linked case,
		// including the ones already reconciled.
		amount += c.AmountMinor
		if c.Currency != "" {
			currency = c.Currency
		}
		if !isOpenStatus(c.Status) {
			continue
		}
		openCount++
		if severityRank(c.Severity) > severityRank(severity) {
			severity = c.Severity
		}
		if oldest.IsZero() || c.CreatedAt.Before(oldest) {
			oldest = c.CreatedAt
		}
		if c.CreatedAt.After(newest) {
			newest = c.CreatedAt
		}
	}
	if openCount == 0 {
		oldest = incident.FirstSeenAt
	}

	age := time.Duration(0)
	if !oldest.IsZero() {
		age = now.Sub(oldest)
	}
	priority := ScorePriority(PriorityInput{
		Severity:     severity,
		AmountMinor:  amount,
		Age:          age,
		IncidentSize: len(cases),
	})

	confidence := 0.6
	if len(cases) > 1 {
		confidence = math.Min(0.95, 0.6+float64(len(cases)-1)*0.07)
	}

	status := incident.Status
	resolvedAt := incident.ResolvedAt
	switch {
	case openCount == 0 && incident.Status != "CLOSED":
		status = "RESOLVED"
		nowResolved := now
		resolvedAt = &nowResolved
	case openCount > 0 && status == "RESOLVED":
		status = "OPEN"
		resolvedAt = nil
	}

	_, err = s.db.Exec(ctx, `
		UPDATE incidents SET
			case_count = $2,
			open_case_count = $3,
			affected_amount_minor = $4,
			currency = $5,
			severity = $6,
			priority_score = $7,
			confidence = $8,
			status = $9,
			first_seen_at = $10,
			last_seen_at = $11,
			ai_summary = $12,
			root_cause_summary = $13,
			next_step = $14,
			resolved_at = $15,
			updated_at = now()
		WHERE id = $1`,
		incidentID, len(cases), openCount, amount, currency, severity, priority, confidence,
		status, oldest, newest,
		incidentSummary(incident.RootCauseCode, incident.RootCauseSummary, len(cases)),
		incident.RootCauseSummary, nextStepFor(incident.RootCauseCode), resolvedAt)
	if err != nil {
		return err
	}
	return nil
}

// RefreshCase recomputes the incidents that contain a case. It runs after a
// case changes state so triage numbers never drift.
func (s *Service) RefreshCase(ctx context.Context, caseID string) error {
	rows, err := s.db.Query(ctx, `
		SELECT incident_id::text FROM incident_cases WHERE case_id = $1`, caseID)
	if err != nil {
		return err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.refreshIncident(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// AddCase links a case to an incident at the merchant's request.
func (s *Service) AddCase(ctx context.Context, merchantID, incidentID, caseID string) error {
	loader := NewFactsLoader(s.db)
	facts, err := loader.LoadOrErr(ctx, caseID)
	if err != nil {
		return err
	}
	if facts.MerchantID != merchantID {
		return errors.New("case belongs to a different merchant")
	}
	if err := facts.VerifySharedFacts(); err != nil {
		return fmt.Errorf("case does not show the shared facts: %w", err)
	}
	if err := s.linkCase(ctx, incidentID, facts, "MERCHANT", 1); err != nil {
		return err
	}
	return s.refreshIncident(ctx, incidentID)
}

// RemoveCase unlinks a case from an incident at the merchant's request. A
// wrong grouping never bypasses per-case policy checks: removing a case only
// changes the triage grouping, never the case's own policy path.
func (s *Service) RemoveCase(ctx context.Context, merchantID, incidentID, caseID string) error {
	tag, err := s.db.Exec(ctx, `
		DELETE FROM incident_cases ic
		USING exception_cases c
		WHERE ic.incident_id = $1 AND ic.case_id = $2 AND c.id = ic.case_id AND c.merchant_id = $3`,
		incidentID, caseID, merchantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("case is not linked to this incident")
	}
	return s.refreshIncident(ctx, incidentID)
}

// RegroupMerchant re-runs grouping for every open case of a merchant. It is
// used after a provider-side fix so the inbox reflects reality again.
func (s *Service) RegroupMerchant(ctx context.Context, merchantID string) error {
	rows, err := s.db.Query(ctx, `
		SELECT id::text FROM exception_cases
		WHERE merchant_id = $1 AND status NOT IN ('RESOLVED','REJECTED','CLOSED')
		ORDER BY created_at ASC`, merchantID)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := s.GroupCase(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// ---- incident queries ----

func (s *Service) loadIncidentRow(ctx context.Context, incidentID string) (*Incident, error) {
	var i Incident
	err := s.db.QueryRow(ctx, `
		SELECT id::text, incident_number, merchant_id::text, COALESCE(provider_account_id::text, ''),
		       fingerprint, root_cause_code, root_cause_summary, ai_summary, next_step, status,
		       severity, confidence, priority_score, case_count, open_case_count,
		       affected_amount_minor, currency, first_seen_at, last_seen_at, resolved_at,
		       created_at, updated_at, draft_type
		FROM incidents WHERE id = $1`, incidentID).Scan(
		&i.ID, &i.Number, &i.MerchantID, &i.ProviderAccountID,
		&i.Fingerprint, &i.RootCauseCode, &i.RootCauseSummary, &i.AISummary, &i.NextStep, &i.Status,
		&i.Severity, &i.Confidence, &i.PriorityScore, &i.CaseCount, &i.OpenCaseCount,
		&i.AffectedAmount, &i.Currency, &i.FirstSeenAt, &i.LastSeenAt, &i.ResolvedAt,
		&i.CreatedAt, &i.UpdatedAt, &i.DraftType)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

const incidentCasesQuery = `
SELECT c.id::text, c.case_number, c.status, c.case_type,
       COALESCE(u.display_name, ''), COALESCE(u.email::text, ''), c.merchant_order_ref,
       COALESCE(pt.provider_status, ''),
       COALESCE(os.order_status, ev.order_status, ''),
       COALESCE(pt.amount_minor, 0), COALESCE(pt.currency, ''),
       c.created_at
FROM incident_cases ic
JOIN exception_cases c ON c.id = ic.case_id
JOIN users u ON u.id = c.customer_id
LEFT JOIN payment_transactions pt ON pt.id = c.payment_transaction_id
LEFT JOIN LATERAL (
	SELECT * FROM order_snapshots s
	WHERE s.merchant_id = c.merchant_id AND s.merchant_order_ref = c.merchant_order_ref
	ORDER BY s.fetched_at DESC LIMIT 1
) os ON true
LEFT JOIN LATERAL (
	SELECT COALESCE(payload->>'Status', payload->>'status') AS order_status
	FROM case_evidence ce
	WHERE ce.case_id = c.id AND ce.evidence_type = 'ORDER_SNAPSHOT'
	ORDER BY ce.fetched_at DESC LIMIT 1
) ev ON true
WHERE ic.incident_id = $1
ORDER BY c.created_at ASC`

func (s *Service) loadIncidentCases(ctx context.Context, incidentID string) ([]IncidentCase, error) {
	rows, err := s.db.Query(ctx, incidentCasesQuery, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now().UTC()
	out := []IncidentCase{}
	for rows.Next() {
		var c IncidentCase
		if err := rows.Scan(&c.CaseID, &c.CaseNumber, &c.Status, &c.CaseType,
			&c.CustomerName, &c.CustomerEmail, &c.MerchantOrderRe,
			&c.ProviderStatus, &c.OrderStatus, &c.AmountMinor, &c.Currency, &c.CreatedAt); err != nil {
			return nil, err
		}
		age := now.Sub(c.CreatedAt)
		c.AgeMinutes = int(age.Minutes())
		c.Severity = severityForCase(c.ProviderStatus, c.OrderStatus, c.AmountMinor, age)
		c.Summary = caseSummary(c.ProviderStatus, c.OrderStatus)
		c.NextStep = caseNextStep(c.ProviderStatus, c.OrderStatus)
		out = append(out, c)
	}
	return out, rows.Err()
}

// severityForCase mirrors the risk engine for triage rows.
func severityForCase(providerStatus, orderStatus string, amountMinor int64, age time.Duration) string {
	out := risk.Derive(risk.Input{
		ProviderStatus:     providerStatus,
		OrderStatus:        orderStatus,
		AmountMismatch:     false,
		CurrencyMismatch:   false,
		EvidenceAgeSeconds: int(age.Seconds()),
	})
	if amountMinor > 0 && out.Severity == "LOW" {
		return "LOW"
	}
	return out.Severity
}

// ListIncidents returns the merchant's incidents, highest priority first.
//
// The grouping threshold decides what counts as a group: an incident with
// fewer linked cases than MinCases is not a shared cause yet, so it stays
// out of the inbox and its cases are listed individually instead.
func (s *Service) ListIncidents(ctx context.Context, merchantID string, includeClosed bool) ([]Incident, error) {
	query := `
		SELECT id::text FROM incidents
		WHERE merchant_id = $1 AND case_count >= $2`
	if !includeClosed {
		query += ` AND status <> 'CLOSED'`
	}
	query += ` ORDER BY priority_score DESC, last_seen_at DESC`

	rows, err := s.db.Query(ctx, query, merchantID, s.cfg.MinCases)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := []Incident{}
	for _, id := range ids {
		incident, err := s.GetIncident(ctx, merchantID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, *incident)
	}
	return out, nil
}

// GetIncident returns one incident with its cases and draft counts.
func (s *Service) GetIncident(ctx context.Context, merchantID, incidentID string) (*Incident, error) {
	incident, err := s.loadIncidentRow(ctx, incidentID)
	if err != nil {
		return nil, err
	}
	if incident.MerchantID != merchantID {
		return nil, ErrNotFound
	}
	cases, err := s.loadIncidentCases(ctx, incidentID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	var (
		amount   int64
		oldest   time.Time
		severity = "LOW"
	)
	for i := range cases {
		c := &cases[i]
		amount += c.AmountMinor
		if !isOpenStatus(c.Status) {
			continue
		}
		if oldest.IsZero() || c.CreatedAt.Before(oldest) {
			oldest = c.CreatedAt
		}
		if severityRank(c.Severity) > severityRank(severity) {
			severity = c.Severity
		}
		c.PriorityScore = ScorePriority(PriorityInput{
			Severity:     c.Severity,
			AmountMinor:  c.AmountMinor,
			Age:          now.Sub(c.CreatedAt),
			IncidentSize: len(cases),
		})
	}
	incident.Cases = cases
	incident.OpenCaseCount = countOpen(cases)
	incident.AffectedAmount = amount
	if severityRank(severity) > severityRank(incident.Severity) {
		incident.Severity = severity
	}
	incident.PriorityScore = ScorePriority(PriorityInput{
		Severity:     incident.Severity,
		AmountMinor:  amount,
		Age:          now.Sub(oldest),
		IncidentSize: len(cases),
	})

	counts := map[string]int{}
	rows, err := s.db.Query(ctx, `
		SELECT status, count(*) FROM message_drafts
		WHERE incident_id = $1 GROUP BY status`, incidentID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			rows.Close()
			return nil, err
		}
		counts[status] = count
	}
	rows.Close()
	incident.DraftsPending = counts["DRAFTED"]
	incident.DraftsApproved = counts["APPROVED"]
	incident.DraftsSent = counts["SENT"]
	return incident, nil
}

// ErrNotFound is returned when an incident does not exist for the merchant.
var ErrNotFound = errors.New("incident not found")

// IncidentForCase returns the incident a case belongs to, or nil when the case
// is not part of a group yet.
func (s *Service) IncidentForCase(ctx context.Context, merchantID, caseID string) (*Incident, error) {
	var incidentID string
	err := s.db.QueryRow(ctx, `
		SELECT i.id::text FROM incidents i
		JOIN incident_cases ic ON ic.incident_id = i.id
		WHERE ic.case_id = $1 AND i.merchant_id = $2
		ORDER BY i.last_seen_at DESC
		LIMIT 1`, caseID, merchantID).Scan(&incidentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	incident, err := s.GetIncident(ctx, merchantID, incidentID)
	if err != nil {
		return nil, err
	}
	return incident, nil
}

// ListTriageCases returns the merchant's cases ranked for the triage inbox.
// Cases already linked to an incident are returned as part of their incident;
// standalone cases appear on their own so nothing disappears from the inbox.
func (s *Service) ListTriageCases(ctx context.Context, merchantID string) ([]IncidentCase, error) {
	rows, err := s.db.Query(ctx, `
		SELECT c.id::text, c.case_number, c.status, c.case_type,
		       COALESCE(u.display_name, ''), COALESCE(u.email::text, ''), c.merchant_order_ref,
		       COALESCE(pt.provider_status, ''), COALESCE(os.order_status, ev.order_status, ''),
		       COALESCE(pt.amount_minor, 0), COALESCE(pt.currency, ''), c.created_at
		FROM exception_cases c
		JOIN users u ON u.id = c.customer_id
		LEFT JOIN payment_transactions pt ON pt.id = c.payment_transaction_id
		LEFT JOIN LATERAL (
			SELECT * FROM order_snapshots s
			WHERE s.merchant_id = c.merchant_id AND s.merchant_order_ref = c.merchant_order_ref
			ORDER BY s.fetched_at DESC LIMIT 1
		) os ON true
		LEFT JOIN LATERAL (
			SELECT COALESCE(payload->>'Status', payload->>'status') AS order_status
			FROM case_evidence ce
			WHERE ce.case_id = c.id AND ce.evidence_type = 'ORDER_SNAPSHOT'
			ORDER BY ce.fetched_at DESC LIMIT 1
		) ev ON true
		WHERE c.merchant_id = $1 AND c.status NOT IN ('RESOLVED','REJECTED','CLOSED')
		  -- A case shown inside an incident card must not also appear as a
		  -- standalone row; below-threshold groups stay out of the inbox, so
		  -- their cases still list individually here.
		  AND NOT EXISTS (
			SELECT 1 FROM incident_cases ic
			JOIN incidents i ON i.id = ic.incident_id
			WHERE ic.case_id = c.id AND i.merchant_id = c.merchant_id
			  AND i.case_count >= $2 AND i.status <> 'CLOSED'
		  )
		ORDER BY c.created_at DESC`, merchantID, s.cfg.MinCases)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now().UTC()
	out := []IncidentCase{}
	for rows.Next() {
		var c IncidentCase
		if err := rows.Scan(&c.CaseID, &c.CaseNumber, &c.Status, &c.CaseType,
			&c.CustomerName, &c.CustomerEmail, &c.MerchantOrderRe,
			&c.ProviderStatus, &c.OrderStatus, &c.AmountMinor, &c.Currency, &c.CreatedAt); err != nil {
			return nil, err
		}
		age := now.Sub(c.CreatedAt)
		c.AgeMinutes = int(age.Minutes())
		c.Severity = severityForCase(c.ProviderStatus, c.OrderStatus, c.AmountMinor, age)
		c.Summary = caseSummary(c.ProviderStatus, c.OrderStatus)
		c.NextStep = caseNextStep(c.ProviderStatus, c.OrderStatus)
		c.PriorityScore = ScorePriority(PriorityInput{
			Severity:     c.Severity,
			AmountMinor:  c.AmountMinor,
			Age:          age,
			IncidentSize: 1,
		})
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- wording helpers ----

func incidentSummary(rootCause, rootSummary string, caseCount int) string {
	if caseCount <= 1 {
		return rootSummary
	}
	return fmt.Sprintf("%s %d customer cases share this cause.", rootSummary, caseCount)
}

func nextStepFor(rootCause string) string {
	switch rootCause {
	case RootCauseOrderUpdateMissing:
		return "Check the PayPal webhook delivery for this account, then reconcile each linked order."
	case RootCauseAmountMismatch:
		return "Compare the captured amount with the order amount on each linked case before reconciling."
	case RootCauseCurrencyMismatch:
		return "Check the settlement currency on the linked orders before reconciling."
	case RootCauseDuplicateCharge:
		return "Review the duplicate captures and decide the refund path with PolicyGuard."
	case RootCausePaymentUnconfirmed:
		return "Wait for the provider to confirm the capture, then re-check the order state."
	default:
		return "Verify the provider and order state on each linked case before taking action."
	}
}

func caseSummary(providerStatus, orderStatus string) string {
	paid := isPaidStatus(providerStatus)
	settled := isPaidStatus(orderStatus)
	switch {
	case paid && !settled:
		return "Payment captured, order pending."
	case paid && settled:
		return "Payment captured and order updated."
	case !paid:
		return "Payment not confirmed yet."
	default:
		return "Order state differs from the payment record."
	}
}

func caseNextStep(providerStatus, orderStatus string) string {
	paid := isPaidStatus(providerStatus)
	settled := isPaidStatus(orderStatus)
	switch {
	case paid && !settled:
		return "Nothing is needed from the customer. Reconcile the order once the cause is fixed."
	case paid && settled:
		return "Nothing is needed."
	default:
		return "We will update this case when the provider confirms the payment."
	}
}

func isPaidStatus(status string) bool {
	switch strings.ToUpper(status) {
	case "COMPLETED", "CAPTURED", "PAID", "FULFILLED", "RECONCILED":
		return true
	}
	return false
}

func isOpenStatus(status string) bool {
	switch strings.ToUpper(status) {
	case "RESOLVED", "REJECTED", "CLOSED":
		return false
	}
	return true
}

func countOpen(cases []IncidentCase) int {
	n := 0
	for _, c := range cases {
		if isOpenStatus(c.Status) {
			n++
		}
	}
	return n
}

func severityRank(severity string) int {
	switch strings.ToUpper(severity) {
	case "CRITICAL":
		return 4
	case "HIGH":
		return 3
	case "MEDIUM":
		return 2
	case "LOW":
		return 1
	}
	return 0
}
