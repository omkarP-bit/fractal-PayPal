package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"fractal/api/internal/auth"
)

type Party struct {
	ID    string `json:"id"`
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

type Payment struct {
	Status          string    `json:"status"`
	AmountMinor     int64     `json:"amount_minor"`
	Currency        string    `json:"currency"`
	Provider        string    `json:"provider,omitempty"`
	ProviderOrderID string    `json:"provider_order_id,omitempty"`
	CaptureID       string    `json:"capture_id,omitempty"`
	FetchedAt       time.Time `json:"fetched_at,omitempty"`
}

type Order struct {
	Ref         string    `json:"ref"`
	Status      string    `json:"status"`
	AmountMinor int64     `json:"amount_minor"`
	Currency    string    `json:"currency"`
	FetchedAt   time.Time `json:"fetched_at,omitempty"`
}

type Finding struct {
	RuleID  string `json:"rule_id,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Passed  bool   `json:"passed"`
}

type Decision struct {
	ID             string    `json:"id"`
	ActionType     string    `json:"action_type,omitempty"`
	Outcome        string    `json:"outcome"`
	PolicyRevision string    `json:"policy_revision"`
	Findings       []Finding `json:"findings"`
	CreatedAt      time.Time `json:"created_at"`
}

type Summary struct {
	ID             string    `json:"id"`
	CaseNumber     string    `json:"case_number"`
	Status         string    `json:"status"`
	CaseType       string    `json:"case_type"`
	OpenedVia      string    `json:"opened_via"`
	Version        int       `json:"version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Merchant       Party     `json:"merchant"`
	Customer       Party     `json:"customer"`
	Payment        *Payment  `json:"payment"`
	Order          *Order    `json:"order"`
	Summary        string    `json:"summary"`
	NextStep       string    `json:"next_step"`
	LatestDecision *Decision `json:"latest_decision"`
}

type Investigation struct {
	Category            string    `json:"category"`
	RootCauseHypothesis string    `json:"root_cause_hypothesis"`
	EvidenceIDs         []string  `json:"evidence_ids"`
	SuggestedSeverity   string    `json:"suggested_severity"`
	ValidationStatus    string    `json:"validation_status,omitempty"`
	Recommendation      string    `json:"recommendation,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
}

type Evidence struct {
	ID           string    `json:"id"`
	EvidenceType string    `json:"evidence_type"`
	SourceSystem string    `json:"source_system"`
	SourceRef    string    `json:"source_ref"`
	FetchedAt    time.Time `json:"fetched_at"`
}

type Activity struct {
	ID         int64          `json:"id"`
	ActorType  string         `json:"actor_type"`
	EventType  string         `json:"event_type"`
	Visibility string         `json:"visibility"`
	Data       map[string]any `json:"data"`
	CreatedAt  time.Time      `json:"created_at"`
}

type Message struct {
	ID         string    `json:"id"`
	AuthorType string    `json:"author_type"`
	Visibility string    `json:"visibility"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

type Approval struct {
	ID         string    `json:"id"`
	ActionType string    `json:"action_type"`
	Decision   string    `json:"decision"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type ProviderInfo struct {
	Name      string    `json:"name"`
	OrderID   string    `json:"order_id"`
	CaptureID string    `json:"capture_id"`
	FetchedAt time.Time `json:"fetched_at"`
}

type Detail struct {
	Summary
	Provider         *ProviderInfo  `json:"provider"`
	Investigation    *Investigation `json:"investigation"`
	Evidence         []Evidence     `json:"evidence"`
	Activity         []Activity     `json:"activity"`
	Messages         []Message      `json:"messages"`
	AvailableActions []string       `json:"available_actions"`
	PendingApprovals []Approval     `json:"pending_approvals"`
}

const summaryColumns = `
	c.id::text, c.case_number, c.status, c.case_type, c.opened_via, c.version,
	c.created_at, c.updated_at,
	m.id::text, m.legal_name,
	u.id::text, u.email::text, u.display_name,
	COALESCE(pt.provider, 'PAYPAL'), COALESCE(pt.provider_order_id, ''), COALESCE(pt.capture_id, ''),
	COALESCE(pt.amount_minor, 0), COALESCE(pt.currency, 'USD'),
	COALESCE(pt.provider_status, 'PENDING'), COALESCE(pt.last_fetched_at, c.created_at),
	COALESCE(os.merchant_order_ref, c.merchant_order_ref),
	COALESCE(os.order_status, CASE WHEN pt.provider_status = 'COMPLETED' THEN 'PAYMENT_PENDING' ELSE 'PENDING' END),
	COALESCE(os.amount_minor, pt.amount_minor, 0),
	COALESCE(os.currency, pt.currency, 'USD'),
	COALESCE(os.fetched_at, c.created_at),
	COALESCE(pd.id::text, ''), COALESCE(pd.action_type, ''), COALESCE(pd.outcome, ''),
	COALESCE(pd.policy_revision, ''), COALESCE(pd.findings, '[]'::jsonb),
	COALESCE(pd.created_at, c.created_at)
`

const summaryFrom = `
FROM exception_cases c
JOIN merchants m ON m.id = c.merchant_id
JOIN users u ON u.id = c.customer_id
LEFT JOIN payment_transactions pt ON pt.id = c.payment_transaction_id
LEFT JOIN LATERAL (
	SELECT * FROM order_snapshots s
	WHERE s.merchant_id = c.merchant_id AND s.merchant_order_ref = c.merchant_order_ref
	ORDER BY s.fetched_at DESC LIMIT 1
) os ON true
LEFT JOIN LATERAL (
	SELECT * FROM policy_decisions d WHERE d.case_id = c.id ORDER BY d.created_at DESC LIMIT 1
) pd ON true
`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSummary(row rowScanner) (Summary, error) {
	var (
		s                           Summary
		merchantName                string
		customerEmail, customerName string
		provider, providerOrderID   string
		captureID                   string
		paymentStatus               string
		paymentAmount               int64
		paymentCurrency             string
		paymentFetchedAt            time.Time
		orderRef, orderStatus       string
		orderAmount                 int64
		orderCurrency               string
		orderFetchedAt              time.Time
		decisionID, decisionAction  string
		decisionOutcome, revision   string
		findings                    []byte
		decisionAt                  time.Time
	)
	err := row.Scan(
		&s.ID, &s.CaseNumber, &s.Status, &s.CaseType, &s.OpenedVia, &s.Version,
		&s.CreatedAt, &s.UpdatedAt,
		&s.Merchant.ID, &merchantName,
		&s.Customer.ID, &customerEmail, &customerName,
		&provider, &providerOrderID, &captureID,
		&paymentAmount, &paymentCurrency, &paymentStatus, &paymentFetchedAt,
		&orderRef, &orderStatus, &orderAmount, &orderCurrency, &orderFetchedAt,
		&decisionID, &decisionAction, &decisionOutcome, &revision, &findings, &decisionAt,
	)
	if err != nil {
		return s, err
	}

	s.Merchant.Name = merchantName
	s.Customer.Email = customerEmail
	s.Customer.Name = customerName

	// Payment is always present in the projection (defaults are applied in SQL).
	s.Payment = &Payment{
		Status:          paymentStatus,
		AmountMinor:     paymentAmount,
		Currency:        paymentCurrency,
		Provider:        provider,
		ProviderOrderID: providerOrderID,
		CaptureID:       captureID,
		FetchedAt:       paymentFetchedAt,
	}
	s.Order = &Order{
		Ref:         orderRef,
		Status:      orderStatus,
		AmountMinor: orderAmount,
		Currency:    orderCurrency,
		FetchedAt:   orderFetchedAt,
	}

	if decisionID != "" {
		d := &Decision{
			ID:             decisionID,
			ActionType:     decisionAction,
			Outcome:        decisionOutcome,
			PolicyRevision: revision,
			CreatedAt:      decisionAt,
			Findings:       []Finding{},
		}
		_ = json.Unmarshal(findings, &d.Findings)
		for i := range d.Findings {
			if d.Findings[i].Code == "GATE_PASSED" || d.Findings[i].Code == "" {
				d.Findings[i].Passed = true
			}
		}
		s.LatestDecision = d
	}

	s.Summary, s.NextStep = narrative(s.Status, paymentStatus, orderStatus, merchantName)
	return s, nil
}

func narrative(status, paymentStatus, orderStatus, merchant string) (string, string) {
	switch {
	case status == "RESOLVED" || status == "CLOSED":
		return "The payment and the store order now agree. FRACTAL kept the full evidence and audit trail for both sides.",
			"No further action is needed."
	case paymentStatus == "COMPLETED" && orderStatus != "PAID":
		return fmt.Sprintf("PayPal confirms the payment, but %s's order system still shows the order as pending. FRACTAL is checking the webhook path and the latest order snapshot.", merchant),
			"Nothing is needed from the customer. Please do not pay again."
	case paymentStatus == "COMPLETED":
		return "PayPal confirms the payment and the store order is up to date.", "Nothing is needed."
	default:
		return "FRACTAL is verifying the payment and order state before deciding the next safe step.",
			"We will update this case when the provider confirms the payment."
	}
}

func (h *Handlers) ListCases(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	if claims == nil {
		writeProblem(w, http.StatusUnauthorized, "Unauthenticated", "Missing session.")
		return
	}

	query := `SELECT` + summaryColumns + summaryFrom
	args := []any{}
	switch {
	case isCustomer(claims):
		query += ` WHERE c.customer_id = $1`
		args = append(args, claims.Sub)
	case isMerchant(claims) && claims.MerchantID != "":
		query += ` WHERE c.merchant_id = $1`
		args = append(args, claims.MerchantID)
	case claims.Kind == "PLATFORM_ADMIN":
		// Platform admins with no merchant scope see every case.
	default:
		writeProblem(w, http.StatusForbidden, "Forbidden", "This account cannot list cases.")
		return
	}
	query += ` ORDER BY c.created_at DESC`

	rows, err := h.DB.Query(r.Context(), query, args...)
	if err != nil {
		h.Log.Error("list cases", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not load cases", "The case list query failed.")
		return
	}
	defer rows.Close()

	out := make([]Summary, 0)
	for rows.Next() {
		item, err := scanSummary(rows)
		if err != nil {
			h.Log.Error("scan case summary", "err", err)
			writeProblem(w, http.StatusInternalServerError, "Could not load cases", "A case row could not be read.")
			return
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		h.Log.Error("iterate cases", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not load cases", "The case list query failed.")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) GetCase(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	caseID, ok := uuidText(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "Case not found", "That case id is not valid.")
		return
	}
	detail, found, err := h.buildDetail(r.Context(), caseID, claims)
	if err != nil {
		h.Log.Error("get case", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not load case", "The case query failed.")
		return
	}
	if !found {
		// Non-parties get 404 so ids cannot be enumerated.
		writeProblem(w, http.StatusNotFound, "Case not found", "This case does not exist or is not shared with you.")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *Handlers) buildDetail(ctx context.Context, caseID string, claims *auth.Claims) (*Detail, bool, error) {
	_, _, party, err := h.partyFor(ctx, caseID, claims)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !party {
		return nil, false, nil
	}

	row := h.DB.QueryRow(ctx, `SELECT`+summaryColumns+summaryFrom+` WHERE c.id = $1`, caseID)
	summary, err := scanSummary(row)
	if err != nil {
		return nil, false, err
	}

	detail := &Detail{
		Summary:          summary,
		Evidence:         []Evidence{},
		Activity:         []Activity{},
		Messages:         []Message{},
		AvailableActions: []string{},
		PendingApprovals: []Approval{},
	}
	if summary.Payment != nil {
		detail.Provider = &ProviderInfo{
			Name:      summary.Payment.Provider,
			OrderID:   summary.Payment.ProviderOrderID,
			CaptureID: summary.Payment.CaptureID,
			FetchedAt: summary.Payment.FetchedAt,
		}
	}

	// Latest investigation.
	var (
		findings        []byte
		evidenceCSV     string
		recommendation  string
		severity        string
		validation      string
		investigationAt time.Time
	)
	err = h.DB.QueryRow(ctx, `
		SELECT findings, COALESCE(recommendation, ''), COALESCE(suggested_severity, ''),
		       COALESCE(array_to_string(evidence_ids, ','), ''), validation_status, created_at
		FROM investigations WHERE case_id = $1 ORDER BY run_number DESC LIMIT 1`, caseID).
		Scan(&findings, &recommendation, &severity, &evidenceCSV, &validation, &investigationAt)
	if err == nil {
		var payload struct {
			Category     string `json:"category"`
			RootCause    string `json:"root_cause_hypothesis"`
			RootCauseAlt string `json:"root_cause"`
			SuggestedFix string `json:"suggested_fix"`
		}
		_ = json.Unmarshal(findings, &payload)
		root := payload.RootCause
		if root == "" {
			root = payload.RootCauseAlt
		}
		if root == "" {
			root = "The provider and merchant states disagree; the webhook delivery path is the likely cause."
		}
		category := payload.Category
		if category == "" {
			category = "PROVIDER_MERCHANT_STATE_MISMATCH"
		}
		recommendationText := recommendation
		if recommendationText == "" {
			recommendationText = payload.SuggestedFix
		}
		ids := []string{}
		for _, id := range strings.Split(evidenceCSV, ",") {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				ids = append(ids, trimmed)
			}
		}
		detail.Investigation = &Investigation{
			Category:            category,
			RootCauseHypothesis: root,
			EvidenceIDs:         ids,
			SuggestedSeverity:   severity,
			ValidationStatus:    validation,
			Recommendation:      recommendationText,
			CreatedAt:           investigationAt,
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, err
	}

	// Evidence.
	evidenceRows, err := h.DB.Query(ctx, `
		SELECT id::text, evidence_type, source_system, source_ref, fetched_at
		FROM case_evidence WHERE case_id = $1 ORDER BY fetched_at ASC`, caseID)
	if err != nil {
		return nil, false, err
	}
	for evidenceRows.Next() {
		var item Evidence
		if err := evidenceRows.Scan(&item.ID, &item.EvidenceType, &item.SourceSystem, &item.SourceRef, &item.FetchedAt); err != nil {
			evidenceRows.Close()
			return nil, false, err
		}
		detail.Evidence = append(detail.Evidence, item)
	}
	evidenceRows.Close()

	// Activity visibility depends on the viewer.
	visibility := []string{"CUSTOMER"}
	if isMerchant(claims) {
		visibility = append(visibility, "MERCHANT")
	}
	if claims != nil && claims.Kind == "PLATFORM_ADMIN" {
		visibility = append(visibility, "INTERNAL")
	}
	activityRows, err := h.DB.Query(ctx, `
		SELECT seq, actor_type, event_type, visibility, data, created_at
		FROM audit_events WHERE case_id = $1 AND visibility = ANY($2)
		ORDER BY seq ASC`, caseID, visibility)
	if err != nil {
		return nil, false, err
	}
	for activityRows.Next() {
		var item Activity
		var raw []byte
		if err := activityRows.Scan(&item.ID, &item.ActorType, &item.EventType, &item.Visibility, &raw, &item.CreatedAt); err != nil {
			activityRows.Close()
			return nil, false, err
		}
		item.Data = map[string]any{}
		_ = json.Unmarshal(raw, &item.Data)
		detail.Activity = append(detail.Activity, item)
	}
	activityRows.Close()

	// Messages.
	messageVisibility := []string{"SHARED"}
	if isMerchant(claims) {
		messageVisibility = append(messageVisibility, "MERCHANT_ONLY")
	}
	messageRows, err := h.DB.Query(ctx, `
		SELECT id::text, author_type, visibility, body, created_at
		FROM case_messages WHERE case_id = $1 AND visibility = ANY($2)
		ORDER BY created_at ASC`, caseID, messageVisibility)
	if err != nil {
		return nil, false, err
	}
	for messageRows.Next() {
		var item Message
		if err := messageRows.Scan(&item.ID, &item.AuthorType, &item.Visibility, &item.Body, &item.CreatedAt); err != nil {
			messageRows.Close()
			return nil, false, err
		}
		detail.Messages = append(detail.Messages, item)
	}
	messageRows.Close()

	// Pending approvals.
	approvalRows, err := h.DB.Query(ctx, `
		SELECT id::text, action_type, decision, expires_at
		FROM approvals WHERE case_id = $1 AND decision = 'PENDING' AND expires_at > now()
		ORDER BY created_at DESC`, caseID)
	if err != nil {
		return nil, false, err
	}
	for approvalRows.Next() {
		var item Approval
		if err := approvalRows.Scan(&item.ID, &item.ActionType, &item.Decision, &item.ExpiresAt); err != nil {
			approvalRows.Close()
			return nil, false, err
		}
		detail.PendingApprovals = append(detail.PendingApprovals, item)
	}
	approvalRows.Close()

	if isMerchant(claims) && summary.Status != "RESOLVED" && summary.Status != "CLOSED" {
		detail.AvailableActions = append(detail.AvailableActions, "RECONCILE_MERCHANT_ORDER", "REQUEST_REFUND")
	}
	if isMerchant(claims) && len(detail.PendingApprovals) > 0 {
		detail.AvailableActions = append(detail.AvailableActions, "DECIDE_APPROVAL")
	}

	return detail, true, nil
}

// ---- timeline ----

func (h *Handlers) Timeline(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	caseID, ok := uuidText(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "Case not found", "That case id is not valid.")
		return
	}
	_, _, party, err := h.partyFor(r.Context(), caseID, claims)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !party) {
		writeProblem(w, http.StatusNotFound, "Case not found", "This case is not shared with you.")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not load timeline", "The timeline query failed.")
		return
	}

	visibility := []string{"CUSTOMER"}
	if isMerchant(claims) {
		visibility = append(visibility, "MERCHANT")
	}
	if claims != nil && claims.Kind == "PLATFORM_ADMIN" {
		visibility = append(visibility, "INTERNAL")
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT seq, actor_type, event_type, visibility, data, created_at
		FROM audit_events WHERE case_id = $1 AND visibility = ANY($2)
		ORDER BY seq ASC`, caseID, visibility)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not load timeline", "The timeline query failed.")
		return
	}
	defer rows.Close()
	out := make([]Activity, 0)
	for rows.Next() {
		var item Activity
		var raw []byte
		if err := rows.Scan(&item.ID, &item.ActorType, &item.EventType, &item.Visibility, &raw, &item.CreatedAt); err != nil {
			writeProblem(w, http.StatusInternalServerError, "Could not load timeline", "A timeline row could not be read.")
			return
		}
		item.Data = map[string]any{}
		_ = json.Unmarshal(raw, &item.Data)
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

// ---- messages ----

func (h *Handlers) Messages(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	caseID, ok := uuidText(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "Case not found", "That case id is not valid.")
		return
	}
	_, _, party, err := h.partyFor(r.Context(), caseID, claims)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !party) {
		writeProblem(w, http.StatusNotFound, "Case not found", "This case is not shared with you.")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not load messages", "The message query failed.")
		return
	}

	visibility := []string{"SHARED"}
	if isMerchant(claims) {
		visibility = append(visibility, "MERCHANT_ONLY")
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT id::text, author_type, visibility, body, created_at
		FROM case_messages WHERE case_id = $1 AND visibility = ANY($2)
		ORDER BY created_at ASC`, caseID, visibility)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not load messages", "The message query failed.")
		return
	}
	defer rows.Close()
	out := make([]Message, 0)
	for rows.Next() {
		var item Message
		if err := rows.Scan(&item.ID, &item.AuthorType, &item.Visibility, &item.Body, &item.CreatedAt); err != nil {
			writeProblem(w, http.StatusInternalServerError, "Could not load messages", "A message row could not be read.")
			return
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) PostMessage(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	caseID, ok := uuidText(r.PathValue("id"))
	if !ok {
		writeProblem(w, http.StatusNotFound, "Case not found", "That case id is not valid.")
		return
	}
	_, _, party, err := h.partyFor(r.Context(), caseID, claims)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !party) {
		writeProblem(w, http.StatusNotFound, "Case not found", "This case is not shared with you.")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not send message", "The case check failed.")
		return
	}

	var req struct {
		Body       string `json:"body"`
		Visibility string `json:"visibility"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid message", "Expected a JSON body with a message.")
		return
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		writeProblem(w, http.StatusBadRequest, "Empty message", "Write something before sending.")
		return
	}
	if len(body) > 4000 {
		writeProblem(w, http.StatusBadRequest, "Message too long", "Messages are limited to 4000 characters.")
		return
	}

	authorType := "CUSTOMER"
	visibility := "SHARED"
	if isMerchant(claims) {
		authorType = "MERCHANT"
		if strings.EqualFold(req.Visibility, "MERCHANT_ONLY") {
			visibility = "MERCHANT_ONLY"
		}
	}

	var (
		messageID string
		createdAt time.Time
	)
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO case_messages (case_id, author_id, author_type, visibility, body)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id::text, created_at`,
		caseID, claims.Sub, authorType, visibility, body).Scan(&messageID, &createdAt)
	if err != nil {
		h.Log.Error("insert message", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not send message", "The message could not be stored.")
		return
	}

	_, _ = h.DB.Exec(r.Context(), `
		INSERT INTO audit_events (case_id, actor_type, actor_id, event_type, visibility, data, event_hash)
		VALUES ($1, $2, $3, 'MESSAGE_POSTED', $4, $5, $6)`,
		caseID, authorType, claims.Sub, visibility, `{"message":"New message added to the case"}`, sha256Hex([]byte(messageID)))

	writeJSON(w, http.StatusCreated, Message{
		ID:         messageID,
		AuthorType: authorType,
		Visibility: visibility,
		Body:       body,
		CreatedAt:  createdAt,
	})
}

// ---- creation ----

type createCaseRequest struct {
	MerchantID       string `json:"merchant_id"`
	MerchantOrderRef string `json:"merchant_order_ref"`
	Claim            struct {
		Email string `json:"email"`
		Token string `json:"token"`
	} `json:"claim"`
	CaseType    string `json:"case_type"`
	Description string `json:"description"`
}

func (h *Handlers) CreateCase(w http.ResponseWriter, r *http.Request) {
	var req createCaseRequest
	if err := decodeJSON(r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request", "Expected the case details as JSON.")
		return
	}

	orderRef := strings.TrimSpace(req.MerchantOrderRef)
	if orderRef == "" {
		writeProblem(w, http.StatusBadRequest, "Missing order", "The merchant order reference is required.")
		return
	}
	caseType := strings.ToUpper(strings.TrimSpace(req.CaseType))
	switch caseType {
	case "PROVIDER_MERCHANT_STATE_MISMATCH", "AMOUNT_MISMATCH", "DUPLICATE_CHARGE", "OTHER":
	default:
		writeProblem(w, http.StatusBadRequest, "Invalid case type", "Choose one of the supported case types.")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Claim.Email))
	token := strings.TrimSpace(req.Claim.Token)
	if email == "" || token == "" {
		writeProblem(w, http.StatusBadRequest, "Ownership proof required", "Email and claim code are both required.")
		return
	}

	merchantFilter := strings.TrimSpace(req.MerchantID)
	if merchantFilter != "" {
		if _, ok := uuidText(merchantFilter); !ok {
			writeProblem(w, http.StatusBadRequest, "Invalid merchant", "The merchant id is not valid.")
			return
		}
	}

	// Resolve the merchant from the claim token when the caller does not send
	// one. That keeps the browser free of hardcoded merchant ids while still
	// proving that the claim code belongs to the order.
	var tokenID, merchantID string
	err := h.DB.QueryRow(r.Context(), `
		SELECT id::text, merchant_id::text FROM order_claim_tokens
		WHERE merchant_order_ref = $1
		  AND customer_email = $2 AND token_hash = $3
		  AND consumed_at IS NULL AND expires_at > now()
		  AND ($4 = '' OR merchant_id::text = $4)
		ORDER BY expires_at DESC
		LIMIT 1`,
		orderRef, email, sha256Hex([]byte(token)), merchantFilter).Scan(&tokenID, &merchantID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusForbidden, "Claim code not valid",
			"That claim code is not valid for this order, or it has expired.")
		return
	}
	if err != nil {
		h.Log.Error("lookup claim token", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not verify the claim",
			"The claim code could not be checked.")
		return
	}

	var customerID, customerName string
	err = h.DB.QueryRow(r.Context(), `
		SELECT id::text, display_name FROM users
		WHERE email = $1 AND kind = 'CUSTOMER' AND status = 'ACTIVE'`, email).
		Scan(&customerID, &customerName)
	if err != nil {
		writeProblem(w, http.StatusNotFound, "Customer not found",
			"No customer account exists for that email.")
		return
	}

	var paymentID *string
	var payID string
	err = h.DB.QueryRow(r.Context(), `
		SELECT id::text FROM payment_transactions
		WHERE merchant_id = $1 AND merchant_order_ref = $2
		ORDER BY created_at DESC LIMIT 1`, merchantID, orderRef).Scan(&payID)
	if err == nil {
		paymentID = &payID
	} else if !errors.Is(err, pgx.ErrNoRows) {
		h.Log.Error("lookup payment", "err", err)
	}

	openedVia := "CUSTOMER"
	description := strings.TrimSpace(req.Description)
	if description == "" {
		description = "Payment issue reported through FRACTAL."
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not open case", "The case transaction could not start.")
		return
	}
	defer tx.Rollback(r.Context())

	var (
		caseID, caseNumber string
		createdAt          time.Time
	)
	err = tx.QueryRow(r.Context(), `
		INSERT INTO exception_cases
			(merchant_id, customer_id, payment_transaction_id, merchant_order_ref, case_type, status, opened_via)
		VALUES ($1, $2, $3, $4, $5, 'OPEN', $6)
		RETURNING id::text, case_number, created_at`,
		merchantID, customerID, paymentID, orderRef, caseType, openedVia).
		Scan(&caseID, &caseNumber, &createdAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeProblem(w, http.StatusConflict, "A case already exists",
				"There is already a live case for this payment and issue type.")
			return
		}
		h.Log.Error("insert case", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not open case", "The case could not be stored.")
		return
	}

	_, _ = tx.Exec(r.Context(), `
		INSERT INTO case_messages (case_id, author_id, author_type, visibility, body)
		VALUES ($1, $2, 'CUSTOMER', 'SHARED', $3)`, caseID, customerID, description)

	_, _ = tx.Exec(r.Context(), `
		INSERT INTO audit_events (case_id, actor_type, actor_id, event_type, visibility, data, event_hash)
		VALUES ($1, 'CUSTOMER', $2, 'CASE_OPENED', 'CUSTOMER', $3, $4)`,
		caseID, customerID,
		fmt.Sprintf(`{"case_number":%q,"merchant_order_ref":%q,"message":"Case opened for you"}`, caseNumber, orderRef),
		sha256Hex([]byte("CASE_OPENED:"+caseID)))

	_, _ = tx.Exec(r.Context(), `UPDATE order_claim_tokens SET consumed_at = now() WHERE id = $1`, tokenID)

	if err := tx.Commit(r.Context()); err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not open case", "The case could not be saved.")
		return
	}

	var providerOrderID string
	_ = h.DB.QueryRow(r.Context(), `
		SELECT COALESCE(provider_order_id, '') FROM payment_transactions WHERE id = $1`, paymentID).
		Scan(&providerOrderID)
	if providerOrderID != "" && h.Collector != nil {
		if _, colErr := h.Collector.Collect(r.Context(), caseID, providerOrderID, orderRef); colErr != nil {
			h.Log.Warn("evidence collection failed", "case_id", caseID, "err", colErr)
		}
	}

	detail, _, err := h.buildDetail(r.Context(), caseID, &auth.Claims{Sub: customerID, Kind: "CUSTOMER"})
	if err != nil {
		// Creation succeeded; return a minimal summary rather than losing the id.
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":          caseID,
			"case_number": caseNumber,
			"status":      "OPEN",
			"created_at":  createdAt,
			"merchant":    Party{ID: merchantID},
			"customer":    Party{ID: customerID, Email: email, Name: customerName},
		})
		return
	}
	writeJSON(w, http.StatusCreated, detail)
}

func decodeJSON(r *http.Request, value any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	return decoder.Decode(value)
}
