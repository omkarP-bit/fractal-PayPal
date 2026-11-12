package assistant

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Draft types. The type decides the autonomy level.
const (
	DraftStatusUpdate     = "STATUS_UPDATE"
	DraftInformationReq   = "INFORMATION_REQUEST"
	DraftResolutionNotice = "RESOLUTION_NOTICE"
)

// Draft statuses.
const (
	DraftStatusDrafted   = "DRAFTED"
	DraftStatusApproved  = "APPROVED"
	DraftStatusSent      = "SENT"
	DraftStatusDiscarded = "DISCARDED"
)

// Guardrail flags recorded on a draft.
const (
	GuardrailMoneyRequest = "CUSTOMER_REQUESTED_MONEY"
	GuardrailNone         = ""
)

// Draft is a merchant-reviewable customer reply built from verified facts.
type Draft struct {
	ID            string     `json:"id"`
	CaseID        string     `json:"case_id"`
	IncidentID    string     `json:"incident_id"`
	DraftType     string     `json:"draft_type"`
	Body          string     `json:"body"`
	Status        string     `json:"status"`
	EvidenceRefs  []string   `json:"evidence_refs"`
	AIGenerated   bool       `json:"ai_generated"`
	GuardrailFlag string     `json:"guardrail_flag"`
	ApprovedBy    *string    `json:"approved_by"`
	ApprovedAt    *time.Time `json:"approved_at"`
	SentAt        *time.Time `json:"sent_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	CaseNumber    string     `json:"case_number,omitempty"`
	CustomerName  string     `json:"customer_name,omitempty"`
	CustomerEmail string     `json:"customer_email,omitempty"`
}

// moneyPattern matches a customer request for money back. The assistant
// never promises a refund or compensation, so a case that asks for one is
// routed to the merchant and PolicyGuard instead of being auto-handled.
var moneyPattern = regexp.MustCompile(`(?i)\b(refund|reimburse|reimbursement|compensation|credit\s+(?:me|back|to)|money\s+back|chargeback|cash\s+back)\b`)

// promisePattern matches wording that promises an outcome or a timeline.
// Drafts that contain it are rejected before they can reach a customer.
var promisePattern = regexp.MustCompile(`(?i)\b(guarantee[ds]?|we\s+will\s+refund|will\s+be\s+refunded|refund\s+(?:has\s+been|was)\s+(?:sent|issued|processed)|within\s+\d+\s*(?:hour|day|week|month)s?|by\s+(?:tomorrow|next\s+week|end\s+of\s+(?:the\s+)?week)|no\s+later\s+than|will\s+be\s+(?:credited|reimbursed|compensated))\b`)

// ContainsMoneyRequest reports whether a customer message asks for money back.
func ContainsMoneyRequest(text string) bool {
	return moneyPattern.MatchString(text)
}

// ContainsPromise reports whether a message promises an outcome or timeline.
func ContainsPromise(text string) bool {
	return promisePattern.MatchString(text)
}

// GuardrailError explains why a draft may not be drafted or sent.
type GuardrailError struct {
	Reason string
}

func (e *GuardrailError) Error() string { return e.Reason }

// CheckDraftBody enforces the assistant guardrails on a draft body. Bodies
// that promise refunds, compensation or timelines are rejected.
func CheckDraftBody(body string) error {
	if strings.TrimSpace(body) == "" {
		return &GuardrailError{Reason: "the draft is empty"}
	}
	if ContainsPromise(body) {
		return &GuardrailError{Reason: "the message promises a refund, compensation or a timeline, which the assistant cannot do"}
	}
	return nil
}

// DraftRequest describes what to draft.
type DraftRequest struct {
	MerchantID string
	CaseID     string
	IncidentID string
	// ActorID is the merchant user on whose behalf the draft is made. It is
	// empty for a fully automatic draft.
	ActorID string
}

// DraftCase builds a draft for one case from its verified facts.
func (s *Service) DraftCase(ctx context.Context, req DraftRequest) (*Draft, error) {
	loader := NewFactsLoader(s.db)
	facts, err := loader.LoadOrErr(ctx, req.CaseID)
	if err != nil {
		return nil, err
	}
	if req.MerchantID != "" && facts.MerchantID != req.MerchantID {
		return nil, ErrNotFound
	}

	var customerName, customerEmail string
	if err := s.db.QueryRow(ctx, `
		SELECT COALESCE(display_name, ''), COALESCE(email::text, '')
		FROM users WHERE id = $1`, facts.CustomerID).Scan(&customerName, &customerEmail); err != nil {
		return nil, err
	}

	// Customer messages are untrusted input. They are read only to detect
	// whether the customer asked for money; their content is never used as
	// instructions and never copied into the draft.
	customerAskedForMoney := s.customerRequestedMoney(ctx, facts.CaseID)

	draftType := DraftStatusUpdate
	guardrail := GuardrailNone
	switch {
	case isSettledStatus(facts) && isResolvedStatus(facts.Status):
		draftType = DraftResolutionNotice
	case !facts.Captured():
		draftType = DraftInformationReq
	}
	if customerAskedForMoney {
		// A money request is never answered with a promise. The draft stays a
		// factual status update and is flagged for merchant handling.
		guardrail = GuardrailMoneyRequest
		if draftType == DraftResolutionNotice {
			draftType = DraftStatusUpdate
		}
	}

	body := RenderDraft(draftType, DraftFacts{
		CustomerName:    customerName,
		OrderRef:        facts.MerchantOrderRe,
		AmountMinor:     facts.ProviderAmount,
		Currency:        facts.Currency,
		ProviderStatus:  facts.ProviderStatus,
		OrderStatus:     facts.OrderStatus,
		Captured:        facts.Captured(),
		AmountMismatch:  facts.AmountMismatch(),
		OrderAmountMinor: facts.OrderAmount,
	})
	if err := CheckDraftBody(body); err != nil {
		return nil, err
	}

	evidence := facts.EvidenceIDs
	if len(evidence) > 20 {
		evidence = evidence[:20]
	}

	return s.insertDraft(ctx, facts, incidentIDOrEmpty(req.IncidentID), draftType, body, evidence, guardrail)
}

func (s *Service) insertDraft(ctx context.Context, facts CaseFacts, incidentID, draftType, body string, evidence []string, guardrail string) (*Draft, error) {
	var incident any
	if incidentID != "" {
		incident = incidentID
	}
	var id string
	err := s.db.QueryRow(ctx, `
		INSERT INTO message_drafts (case_id, incident_id, draft_type, body, status, evidence_refs, ai_generated, guardrail_flag)
		VALUES ($1, $2, $3, $4, 'DRAFTED', $5, true, $6)
		ON CONFLICT (case_id, COALESCE(incident_id, '00000000-0000-0000-0000-000000000000'::uuid), draft_type)
			WHERE status IN ('DRAFTED','APPROVED')
		DO UPDATE SET body = EXCLUDED.body, evidence_refs = EXCLUDED.evidence_refs,
			guardrail_flag = EXCLUDED.guardrail_flag, updated_at = now()
		RETURNING id::text`,
		facts.CaseID, incident, draftType, body, evidence, guardrail).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetDraft(ctx, facts.MerchantID, id)
}

// DraftIncident drafts a reply for every linked case of an incident.
func (s *Service) DraftIncident(ctx context.Context, merchantID, incidentID, actorID string) ([]Draft, error) {
	incident, err := s.GetIncident(ctx, merchantID, incidentID)
	if err != nil {
		return nil, err
	}
	out := []Draft{}
	for _, c := range incident.Cases {
		draft, err := s.DraftCase(ctx, DraftRequest{
			MerchantID: merchantID,
			CaseID:     c.CaseID,
			IncidentID: incidentID,
			ActorID:    actorID,
		})
		if err != nil {
			var guard *GuardrailError
			if errors.As(err, &guard) {
				// A guarded draft is skipped, not silently reworded.
				continue
			}
			return nil, err
		}
		out = append(out, *draft)
	}
	return out, nil
}

// DraftFacts are the verified values a template is filled with.
type DraftFacts struct {
	CustomerName    string
	OrderRef        string
	AmountMinor     int64
	Currency        string
	ProviderStatus  string
	OrderStatus     string
	Captured        bool
	AmountMismatch  bool
	OrderAmountMinor int64
}

// RenderDraft fills a template with verified facts. The templates never state
// an outcome, a refund or a timeline.
func RenderDraft(draftType string, f DraftFacts) string {
	name := strings.TrimSpace(f.CustomerName)
	if name == "" {
		name = "there"
	} else {
		name = strings.Split(name, " ")[0]
	}

	orderRef := strings.TrimSpace(f.OrderRef)
	if orderRef == "" {
		orderRef = "your order"
	}
	amount := ""
	if f.AmountMinor > 0 {
		amount = Money(f.AmountMinor, f.Currency)
	}

	switch draftType {
	case DraftResolutionNotice:
		return fmt.Sprintf(
			"Hi %s,\n\nOur records now show order %s as confirmed, and the captured payment of %s is matched to it. We have closed the exception on your case.",
			name, orderRef, amount)
	case DraftInformationReq:
		return fmt.Sprintf(
			"Hi %s,\n\nWe are checking order %s. To match the payment to your order we need the email address used for the purchase. Please reply here with that email address so we can continue the check.",
			name, orderRef)
	default:
		if !f.Captured {
			return fmt.Sprintf(
				"Hi %s,\n\nWe are still verifying the payment for order %s with the payment provider. Nothing is needed from you right now, and we will update this case as soon as the provider confirms the state.",
				name, orderRef)
		}
		body := fmt.Sprintf(
			"Hi %s,\n\nWe can see that the payment of %s for order %s was captured by the provider, but our order system has not applied that update yet. Our team is checking why the update did not arrive. You do not need to pay again, and we will keep this case updated while we confirm the next step.",
			name, amount, orderRef)
		if f.AmountMismatch && f.OrderAmountMinor > 0 {
			// State the verified difference, never a conclusion about money.
			body += fmt.Sprintf(
				"\n\nThe captured amount and the amount recorded on your order do not match, so we are checking that difference before the order is updated.",
			)
		}
		return body
	}
}

// customerRequestedMoney checks whether the customer asked for money back.
// The message text is only classified, never executed or echoed.
func (s *Service) customerRequestedMoney(ctx context.Context, caseID string) bool {
	rows, err := s.db.Query(ctx, `
		SELECT body FROM case_messages
		WHERE case_id = $1 AND author_type = 'CUSTOMER'
		ORDER BY created_at ASC LIMIT 20`, caseID)
	if err != nil {
		return false
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return found
		}
		if ContainsMoneyRequest(body) {
			found = true
		}
	}
	return found
}

// GetDraft returns one draft scoped to a merchant.
func (s *Service) GetDraft(ctx context.Context, merchantID, draftID string) (*Draft, error) {
	var d Draft
	var incidentID *string
	var approvedBy *string
	err := s.db.QueryRow(ctx, `
		SELECT d.id::text, d.case_id::text, d.incident_id::text, d.draft_type, d.body, d.status,
		       d.evidence_refs::text[], d.ai_generated, d.guardrail_flag, d.approved_by::text,
		       d.approved_at, d.sent_at, d.created_at, d.updated_at,
		       c.case_number, COALESCE(u.display_name, ''), COALESCE(u.email::text, '')
		FROM message_drafts d
		JOIN exception_cases c ON c.id = d.case_id
		JOIN users u ON u.id = c.customer_id
		WHERE d.id = $1 AND c.merchant_id = $2`, draftID, merchantID).Scan(
		&d.ID, &d.CaseID, &incidentID, &d.DraftType, &d.Body, &d.Status,
		&d.EvidenceRefs, &d.AIGenerated, &d.GuardrailFlag, &approvedBy,
		&d.ApprovedAt, &d.SentAt, &d.CreatedAt, &d.UpdatedAt,
		&d.CaseNumber, &d.CustomerName, &d.CustomerEmail)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if incidentID != nil {
		d.IncidentID = *incidentID
	}
	if approvedBy != nil {
		d.ApprovedBy = approvedBy
	}
	if d.EvidenceRefs == nil {
		d.EvidenceRefs = []string{}
	}
	return &d, nil
}

// ListDrafts returns the drafts of an incident or of one case.
func (s *Service) ListDrafts(ctx context.Context, merchantID, incidentID, caseID string) ([]Draft, error) {
	query := `
		SELECT d.id::text, d.case_id::text, d.incident_id::text, d.draft_type, d.body, d.status,
		       d.evidence_refs::text[], d.ai_generated, d.guardrail_flag, d.approved_by::text,
		       d.approved_at, d.sent_at, d.created_at, d.updated_at,
		       c.case_number, COALESCE(u.display_name, ''), COALESCE(u.email::text, '')
		FROM message_drafts d
		JOIN exception_cases c ON c.id = d.case_id
		JOIN users u ON u.id = c.customer_id
		WHERE c.merchant_id = $1`
	args := []any{merchantID}
	if incidentID != "" {
		args = append(args, incidentID)
		query += fmt.Sprintf(" AND d.incident_id = $%d", len(args))
	}
	if caseID != "" {
		args = append(args, caseID)
		query += fmt.Sprintf(" AND d.case_id = $%d", len(args))
	}
	query += " ORDER BY d.created_at ASC"

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Draft{}
	for rows.Next() {
		var d Draft
		var incident, approvedBy *string
		if err := rows.Scan(&d.ID, &d.CaseID, &incident, &d.DraftType, &d.Body, &d.Status,
			&d.EvidenceRefs, &d.AIGenerated, &d.GuardrailFlag, &approvedBy,
			&d.ApprovedAt, &d.SentAt, &d.CreatedAt, &d.UpdatedAt,
			&d.CaseNumber, &d.CustomerName, &d.CustomerEmail); err != nil {
			return nil, err
		}
		if incident != nil {
			d.IncidentID = *incident
		}
		if approvedBy != nil {
			d.ApprovedBy = approvedBy
		}
		if d.EvidenceRefs == nil {
			d.EvidenceRefs = []string{}
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateDraft lets a merchant edit a drafted reply. The guardrails still
// apply to the edited body.
func (s *Service) UpdateDraft(ctx context.Context, merchantID, draftID, body string) (*Draft, error) {
	body = strings.TrimSpace(body)
	if len(body) > 4000 {
		return nil, &GuardrailError{Reason: "the message is longer than 4000 characters"}
	}
	if err := CheckDraftBody(body); err != nil {
		return nil, err
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE message_drafts d SET body = $3, updated_at = now()
		FROM exception_cases c
		WHERE d.id = $1 AND c.id = d.case_id AND c.merchant_id = $2
		  AND d.status IN ('DRAFTED','APPROVED')`,
		draftID, merchantID, body)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.GetDraft(ctx, merchantID, draftID)
}

// ApproveDraft approves one draft.
func (s *Service) ApproveDraft(ctx context.Context, merchantID, draftID, actorID string) (*Draft, error) {
	tag, err := s.db.Exec(ctx, `
		UPDATE message_drafts d SET status = 'APPROVED', approved_by = $3, approved_at = now(), updated_at = now()
		FROM exception_cases c
		WHERE d.id = $1 AND c.id = d.case_id AND c.merchant_id = $2 AND d.status = 'DRAFTED'`,
		draftID, merchantID, actorID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrNotFound
	}
	return s.GetDraft(ctx, merchantID, draftID)
}

// ApproveIncidentDrafts approves every drafted reply of an incident.
func (s *Service) ApproveIncidentDrafts(ctx context.Context, merchantID, incidentID, actorID string) ([]Draft, error) {
	drafts, err := s.ListDrafts(ctx, merchantID, incidentID, "")
	if err != nil {
		return nil, err
	}
	out := []Draft{}
	var failed []string
	for _, d := range drafts {
		if d.Status != DraftStatusDrafted {
			continue
		}
		approved, err := s.ApproveDraft(ctx, merchantID, d.ID, actorID)
		if err != nil {
			failed = append(failed, d.ID)
			continue
		}
		out = append(out, *approved)
	}
	if len(failed) > 0 {
		return out, fmt.Errorf("%d replies could not be approved", len(failed))
	}
	return out, nil
}

// SendResult reports the outcome of sending a draft.
type SendResult struct {
	DraftID   string `json:"draft_id"`
	MessageID string `json:"message_id"`
	CaseID    string `json:"case_id"`
	CaseNumber string `json:"case_number"`
	SentAt    time.Time `json:"sent_at"`
	AutoSent  bool     `json:"auto_sent"`
}

// SendDraft delivers an approved draft to the customer.
//
// The message is stored on the case thread, labeled AI-assisted, and written
// to the audit timeline. The assistant cannot execute actions: this path only
// sends a message.
func (s *Service) SendDraft(ctx context.Context, merchantID, draftID, actorID string) (*SendResult, error) {
	draft, err := s.GetDraft(ctx, merchantID, draftID)
	if err != nil {
		return nil, err
	}
	if draft.Status != DraftStatusApproved {
		return nil, &GuardrailError{Reason: "the draft has not been approved yet"}
	}
	if err := CheckDraftBody(draft.Body); err != nil {
		return nil, err
	}
	return s.deliver(ctx, merchantID, draft, actorID, false)
}

// SendIncidentDrafts delivers every approved draft of an incident.
func (s *Service) SendIncidentDrafts(ctx context.Context, merchantID, incidentID, actorID string) ([]SendResult, error) {
	drafts, err := s.ListDrafts(ctx, merchantID, incidentID, "")
	if err != nil {
		return nil, err
	}
	out := []SendResult{}
	var failed int
	for _, d := range drafts {
		if d.Status != DraftStatusApproved {
			continue
		}
		result, err := s.deliver(ctx, merchantID, &d, actorID, false)
		if err != nil {
			failed++
			continue
		}
		out = append(out, *result)
	}
	if failed > 0 {
		return out, fmt.Errorf("%d replies could not be sent", failed)
	}
	return out, nil
}

// AutoSendEligible reports whether a draft may be sent without a merchant
// approval. Only a factual status update qualifies.
func (s *Service) AutoSendEligible(draft *Draft) bool {
	if !s.cfg.AutoSend {
		return false
	}
	if draft == nil {
		return false
	}
	if draft.DraftType != DraftStatusUpdate {
		return false
	}
	if draft.GuardrailFlag != GuardrailNone {
		return false
	}
	return CheckDraftBody(draft.Body) == nil
}

// deliver writes the customer message, marks the draft sent and records the
// audit event.
func (s *Service) deliver(ctx context.Context, merchantID string, draft *Draft, actorID string, auto bool) (*SendResult, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var caseNumber string
	if err := tx.QueryRow(ctx, `
		SELECT c.case_number FROM message_drafts d
		JOIN exception_cases c ON c.id = d.case_id
		WHERE d.id = $1 AND c.merchant_id = $2`, draft.ID, merchantID).Scan(&caseNumber); err != nil {
		return nil, err
	}

	var messageID string
	var sentAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO case_messages
			(case_id, author_id, author_type, visibility, body, ai_generated, approved_by, sent_at, draft_id)
		VALUES ($1, NULLIF($2, '')::uuid, 'AGENT', 'SHARED', $3, true, NULLIF($4, '')::uuid, now(), $5)
		RETURNING id::text, sent_at`,
		draft.CaseID, actorID, draft.Body, actorID, draft.ID).Scan(&messageID, &sentAt)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		UPDATE message_drafts SET status = 'SENT', sent_at = now(), updated_at = now()
		WHERE id = $1`, draft.ID); err != nil {
		return nil, err
	}

	actorType := "AGENT"
	if actorID != "" {
		actorType = "MERCHANT_USER"
	}
	label := "AI-assisted message"
	if auto {
		label = "AI-assisted status update sent automatically"
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_events (case_id, incident_id, actor_type, actor_id, event_type, visibility, data, event_hash)
		VALUES ($1, NULLIF($2, '')::uuid, $3, NULLIF($4, '')::uuid, 'AI_MESSAGE_SENT', 'CUSTOMER', $5, $6)`,
		draft.CaseID, draft.IncidentID, actorType, actorID,
		fmt.Sprintf(`{"label":%q,"ai_generated":true,"draft_id":%q,"draft_type":%q,"approved_by":%q,"message":"%s"}`,
			label, draft.ID, draft.DraftType, actorID, label),
		hashEvent("AI_MESSAGE_SENT:"+messageID)); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &SendResult{
		DraftID:    draft.ID,
		MessageID:  messageID,
		CaseID:     draft.CaseID,
		CaseNumber: caseNumber,
		SentAt:     sentAt,
		AutoSent:   auto,
	}, nil
}

// DiscardDraft drops a drafted reply.
func (s *Service) DiscardDraft(ctx context.Context, merchantID, draftID string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE message_drafts d SET status = 'DISCARDED', updated_at = now()
		FROM exception_cases c
		WHERE d.id = $1 AND c.id = d.case_id AND c.merchant_id = $2 AND d.status = 'DRAFTED'`,
		draftID, merchantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func incidentIDOrEmpty(value string) string {
	return strings.TrimSpace(value)
}

func isSettledStatus(f CaseFacts) bool {
	return f.OrderSettled()
}

func isResolvedStatus(status string) bool {
	switch strings.ToUpper(status) {
	case "RESOLVED", "CLOSED":
		return true
	}
	return false
}
