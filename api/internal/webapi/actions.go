package webapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"fractal/api/internal/policy"
	"fractal/api/internal/risk"
)

type executionInfo struct {
	ID          string     `json:"id"`
	Status      string     `json:"status"`
	CompletedAt *time.Time `json:"completed_at"`
}

type approvalInfo struct {
	ID           string `json:"id"`
	Decision     string `json:"decision"`
	ParamsSHA256 string `json:"params_sha256"`
}

type caseState struct {
	Status  string `json:"status"`
	Version int    `json:"version"`
}

type actionResponse struct {
	Decision  *Decision      `json:"decision"`
	Execution *executionInfo `json:"execution"`
	Approval  *approvalInfo  `json:"approval"`
	Case      caseState      `json:"case"`
}

type actionRequest struct {
	ActionType string `json:"action_type"`
	Params     struct {
		AmountMinor int64  `json:"amount_minor"`
		Currency    string `json:"currency"`
	} `json:"params"`
}

type caseActionState struct {
	Status          string
	Version         int
	OrderRef        string
	MerchantID      string
	Provider        string
	ProviderOrderID string
	CaptureID       string
	AmountMinor     int64
	Currency        string
	PaymentStatus   string
	OrderStatus     string
	OrderAmount     int64
	OrderCurrency   string
}

func (h *Handlers) loadActionState(ctx context.Context, caseID string) (caseActionState, error) {
	var state caseActionState
	err := h.DB.QueryRow(ctx, `
		SELECT c.status, c.version, c.merchant_order_ref, c.merchant_id::text,
		       COALESCE(pt.provider, 'PAYPAL'), COALESCE(pt.provider_order_id, ''), COALESCE(pt.capture_id, ''),
		       COALESCE(pt.amount_minor, 0), COALESCE(pt.currency, 'USD'), COALESCE(pt.provider_status, ''),
		       COALESCE(os.order_status, ''), COALESCE(os.amount_minor, 0), COALESCE(os.currency, '')
		FROM exception_cases c
		LEFT JOIN payment_transactions pt ON pt.id = c.payment_transaction_id
		LEFT JOIN LATERAL (
			SELECT * FROM order_snapshots s
			WHERE s.merchant_id = c.merchant_id AND s.merchant_order_ref = c.merchant_order_ref
			ORDER BY s.fetched_at DESC LIMIT 1
		) os ON true
		WHERE c.id = $1`, caseID).Scan(
		&state.Status, &state.Version, &state.OrderRef, &state.MerchantID,
		&state.Provider, &state.ProviderOrderID, &state.CaptureID,
		&state.AmountMinor, &state.Currency, &state.PaymentStatus,
		&state.OrderStatus, &state.OrderAmount, &state.OrderCurrency,
	)
	return state, err
}

func (h *Handlers) fetchMerchantOrder(ctx context.Context, ref string) (status string, amountMinor int64, currency string, fetchedAt time.Time, err error) {
	url := fmt.Sprintf("%s/api/orders/%s", h.MerchantStoreBase, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, "", time.Time{}, err
	}
	resp, err := h.HTTP.Do(req)
	if err != nil {
		return "", 0, "", time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, "", time.Time{}, fmt.Errorf("merchant order API returned %d", resp.StatusCode)
	}
	var raw struct {
		Status      string `json:"status"`
		AmountMinor int64  `json:"amount_minor"`
		Currency    string `json:"currency"`
		UpdatedAt   string `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", 0, "", time.Time{}, err
	}
	fetchedAt, err = time.Parse(time.RFC3339, raw.UpdatedAt)
	if err != nil {
		fetchedAt = time.Now().UTC()
	}
	return raw.Status, raw.AmountMinor, raw.Currency, fetchedAt, nil
}

func (h *Handlers) PostAction(w http.ResponseWriter, r *http.Request) {
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
		writeProblem(w, http.StatusInternalServerError, "Could not load case", "The case check failed.")
		return
	}
	if !isMerchant(claims) {
		writeProblem(w, http.StatusForbidden, "Merchant action required",
			"Only a merchant team member can execute resolution actions.")
		return
	}

	var req actionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid action", "Expected a JSON action request.")
		return
	}
	action := strings.ToUpper(strings.TrimSpace(req.ActionType))
	switch action {
	case "RECONCILE_MERCHANT_ORDER":
		h.reconcile(w, r, caseID, req)
	case "REQUEST_REFUND":
		h.requestRefund(w, r, caseID, req)
	default:
		writeProblem(w, http.StatusBadRequest, "Unsupported action", "That action type is not supported.")
	}
}

func (h *Handlers) reconcile(w http.ResponseWriter, r *http.Request, caseID string, req actionRequest) {
	ctx := r.Context()
	claims := claimsFrom(r)
	state, err := h.loadActionState(ctx, caseID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "Case not found", "This case does not exist.")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not load case", "The case state could not be read.")
		return
	}
	if state.Status == "RESOLVED" || state.Status == "CLOSED" {
		writeProblem(w, http.StatusConflict, "Already resolved", "This case is already closed.")
		return
	}
	if state.OrderStatus == "PAID" {
		writeProblem(w, http.StatusConflict, "Order already paid", "The merchant order is already marked paid.")
		return
	}

	freshTx, err := h.PayPal.GetTransaction(ctx, state.ProviderOrderID)
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "Provider unavailable",
			"PayPal could not be reached for the final freshness check. Nothing was executed.")
		return
	}
	freshOrderStatus, freshOrderAmount, freshOrderCurrency, freshOrderFetchedAt, err := h.fetchMerchantOrder(ctx, state.OrderRef)
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "Merchant unavailable",
			"The merchant order API could not be reached. Nothing was executed.")
		return
	}

	params := paramsJSON(freshTx.AmountMinor, freshTx.Currency)
	paramsSHA := sha256Hex(params)
	idempotencyKey := fmt.Sprintf("reconcile-%s-%s", caseID, paramsSHA[:12])

	var execID, execStatus string
	err = h.DB.QueryRow(ctx, `
		SELECT id::text, status FROM action_executions
		WHERE idempotency_key = $1`, idempotencyKey).Scan(&execID, &execStatus)
	switch {
	case err == nil && execStatus == "SUCCEEDED":
		var currentStatus string
		var currentVersion int
		_ = h.DB.QueryRow(ctx, `SELECT status, version FROM exception_cases WHERE id = $1`, caseID).
			Scan(&currentStatus, &currentVersion)
		writeJSON(w, http.StatusOK, actionResponse{
			Execution: &executionInfo{ID: execID, Status: execStatus},
			Case:      caseState{Status: currentStatus, Version: currentVersion},
		})
		return
	case err == nil && execStatus == "IN_PROGRESS":
		writeProblem(w, http.StatusConflict, "Action in progress",
			"This reconciliation is already running. Refresh the case in a moment.")
		return
	case err == nil && execStatus == "FAILED":
		if _, updateErr := h.DB.Exec(ctx, `
			UPDATE action_executions
			SET status = 'IN_PROGRESS', error = NULL, completed_at = NULL
			WHERE id = $1`, execID); updateErr != nil {
			h.Log.Error("reset execution", "err", updateErr)
			writeProblem(w, http.StatusInternalServerError, "Execution record failed",
				"The previous attempt could not be retried.")
			return
		}
	case errors.Is(err, pgx.ErrNoRows):
		execID = ""
	default:
		h.Log.Error("lookup execution", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not check execution",
			"The execution state could not be read.")
		return
	}

	evalID, err := uuid.NewRandom()
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not build decision", "The evaluation id could not be created.")
		return
	}
	evaluatedAt := time.Now().UTC().Format(time.RFC3339)

	riskIn := risk.Input{
		ProviderStatus:      freshTx.Status,
		OrderStatus:         freshOrderStatus,
		AmountMismatch:      freshTx.AmountMinor != freshOrderAmount,
		CurrencyMismatch:    freshTx.Currency != freshOrderCurrency,
		EvidenceAgeSeconds:  int(time.Since(freshTx.FetchedAt).Seconds()),
		ProviderUnreachable: false,
	}
	riskOut := risk.Derive(riskIn)

	in := policy.NewInput()
	in.Request = policy.Request{
		EvaluationID: evalID.String(),
		EvaluatedAt:  evaluatedAt,
		Action: policy.Action{
			Type:           "RECONCILE_MERCHANT_ORDER",
			Params:         map[string]any{"amount_minor": freshTx.AmountMinor, "currency": freshTx.Currency},
			ParamsSHA256:   paramsSHA,
			IdempotencyKey: idempotencyKey,
		},
	}
	in.Case = policy.Case{ID: caseID, Status: state.Status}
	in.Actor = policy.Actor{
		ID:          claims.Sub,
		Type:        claims.Kind,
		MerchantID:  claims.MerchantID,
		Permissions: claims.Permissions,
	}
	in.Merchant = policy.Merchant{ID: state.MerchantID, Active: true}
	in.Facts = policy.Facts{
		Provider: policy.ProviderFact{
			Name:        "PAYPAL",
			Status:      freshTx.Status,
			AmountMinor: freshTx.AmountMinor,
			Currency:    freshTx.Currency,
			Source:      "PAYPAL_API",
			FetchedAt:   freshTx.FetchedAt.Format(time.RFC3339),
		},
		Order: policy.OrderFact{
			Status:      freshOrderStatus,
			AmountMinor: freshOrderAmount,
			Currency:    freshOrderCurrency,
			Source:      "MERCHANT_API",
			FetchedAt:   freshOrderFetchedAt.Format(time.RFC3339),
		},
		Webhook: policy.WebhookFact{SignatureVerified: true, EventCount: 1},
	}
	in.Execution = policy.Execution{AlreadyExecuted: execID != ""}
	in.Risk = policy.Risk{
		Severity:     riskOut.Severity,
		AnomalyScore: nil,
	}

	decision, err := h.Policy.Evaluate(ctx, in)
	if err != nil {
		h.Log.Warn("policy evaluation failed closed", "case", caseID, "err", err)
	}

	findingsJSON, _ := json.Marshal(decision.Findings)
	inputJSON, _ := json.Marshal(in)
	inputSHA := sha256Hex(inputJSON)
	policyRevision := h.policyRevision(ctx)

	var decisionID string
	var decisionAt time.Time
	err = h.DB.QueryRow(ctx, `
		INSERT INTO policy_decisions
			(case_id, evaluation_id, action_type, params_sha256, input, input_sha256,
			 outcome, findings, policy_revision, requested_by)
		VALUES ($1, $2, 'RECONCILE_MERCHANT_ORDER', $3, $4, $5,
		        $6, $7, $8, $9)
		RETURNING id::text, created_at`,
		caseID, evalID.String(), paramsSHA, string(inputJSON), inputSHA,
		decision.Outcome, string(findingsJSON), policyRevision, claims.Sub).Scan(&decisionID, &decisionAt)
	if err != nil {
		h.Log.Error("insert policy decision", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Policy record failed", "The decision could not be recorded; no action was executed.")
		return
	}

	decisionOut := &Decision{
		ID:             decisionID,
		ActionType:     "RECONCILE_MERCHANT_ORDER",
		Outcome:        decision.Outcome,
		PolicyRevision: policyRevision,
		Findings:       findingsFromPolicy(decision.Findings),
		CreatedAt:      decisionAt,
	}

	switch decision.Outcome {
	case "ALLOW":
		if execID == "" {
			err = h.DB.QueryRow(ctx, `
				INSERT INTO action_executions
					(case_id, policy_decision_id, action_type, params, params_sha256, idempotency_key, status)
				VALUES ($1, $2, 'RECONCILE_MERCHANT_ORDER', $3, $4, $5, 'IN_PROGRESS')
				RETURNING id::text`,
				caseID, decisionID, string(params), paramsSHA, idempotencyKey).Scan(&execID)
			if err != nil {
				h.Log.Error("insert execution", "err", err)
				writeProblem(w, http.StatusInternalServerError, "Execution record failed", "The action could not be recorded; nothing was executed.")
				return
			}
		} else {
			if _, err = h.DB.Exec(ctx, `UPDATE action_executions SET policy_decision_id = $2 WHERE id = $1`, execID, decisionID); err != nil {
				h.Log.Error("attach execution decision", "err", err)
			}
		}

		body, _ := json.Marshal(map[string]any{
			"provider":          state.Provider,
			"provider_order_id": state.ProviderOrderID,
			"capture_id":        state.CaptureID,
			"amount_minor":      freshTx.AmountMinor,
			"currency":          freshTx.Currency,
		})
		url := fmt.Sprintf("%s/api/orders/%s/reconcile", h.MerchantStoreBase, state.OrderRef)
		merchantReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		merchantReq.Header.Set("Content-Type", "application/json")
		merchantReq.Header.Set("Idempotency-Key", idempotencyKey)
		merchantReq.Header.Set("X-Fractal-Signature", "dev-mode")

		resp, callErr := h.HTTP.Do(merchantReq)
		var responseBody []byte
		if callErr == nil {
			defer resp.Body.Close()
			responseBody, _ = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		}

		if callErr != nil || resp == nil || resp.StatusCode >= 400 {
			reason := "merchant order API did not accept the reconciliation"
			if callErr != nil {
				reason = callErr.Error()
			} else {
				reason = fmt.Sprintf("merchant order API returned %d", resp.StatusCode)
			}
			_, _ = h.DB.Exec(ctx, `
				UPDATE action_executions
				SET status = 'FAILED', error = $2, completed_at = now()
				WHERE id = $1`, execID, reason)
			_, _ = h.DB.Exec(ctx, `
				INSERT INTO audit_events (case_id, actor_type, actor_id, event_type, visibility, data, event_hash)
				VALUES ($1, 'MERCHANT_USER', $2, 'ACTION_FAILED', 'MERCHANT', $3, $4)`,
				caseID, claims.Sub,
				fmt.Sprintf(`{"action":"RECONCILE_MERCHANT_ORDER","error":%q}`, reason),
				sha256Hex([]byte("ACTION_FAILED:"+execID)))
			h.Log.Warn("merchant reconcile failed", "case", caseID, "err", reason)
			writeJSON(w, http.StatusBadGateway, actionResponse{
				Decision: decisionOut,
				Execution: &executionInfo{ID: execID, Status: "FAILED"},
				Case:     caseState{Status: state.Status, Version: state.Version},
			})
			return
		}

		if !json.Valid(responseBody) {
			wrapped, _ := json.Marshal(map[string]string{"raw": string(responseBody)})
			responseBody = wrapped
		}

		var newVersion int
		err = h.DB.QueryRow(ctx, `
			UPDATE exception_cases
			SET status = 'RESOLVED', resolution_code = 'RECONCILED', closed_at = now(), version = version + 1
			WHERE id = $1
			RETURNING version`, caseID).Scan(&newVersion)
		if err != nil {
			h.Log.Error("close case", "err", err)
			writeProblem(w, http.StatusInternalServerError, "Case update failed", "The order was reconciled but the case could not be closed.")
			return
		}
		_, _ = h.DB.Exec(ctx, `
			UPDATE action_executions
			SET status = 'SUCCEEDED', response = $2, completed_at = now()
			WHERE id = $1`, execID, string(responseBody))

		_, _ = h.DB.Exec(ctx, `
			INSERT INTO order_snapshots
				(merchant_id, merchant_order_ref, amount_minor, currency, order_status, fetched_at, payload_sha256)
			VALUES ($1, $2, $3, $4, 'PAID', now(), $5)`,
			state.MerchantID, state.OrderRef, freshTx.AmountMinor, freshTx.Currency, sha256Hex(responseBody))

		_, _ = h.DB.Exec(ctx, `
			INSERT INTO audit_events (case_id, actor_type, actor_id, event_type, visibility, data, event_hash)
			VALUES ($1, 'SYSTEM', 'executor', 'CASE_RECONCILED', 'CUSTOMER', $2, $3)`,
			caseID,
			`{"action_type":"RECONCILE_MERCHANT_ORDER","outcome":"ALLOW","message":"The store order now matches the verified PayPal payment"}`,
			sha256Hex([]byte("CASE_RECONCILED:"+execID)))

		completed := time.Now().UTC()
		writeJSON(w, http.StatusOK, actionResponse{
			Decision:  decisionOut,
			Execution: &executionInfo{ID: execID, Status: "SUCCEEDED", CompletedAt: &completed},
			Case:      caseState{Status: "RESOLVED", Version: newVersion},
		})
		return

	case "REQUIRE_APPROVAL":
		_, _ = h.DB.Exec(ctx, `
			UPDATE exception_cases
			SET status = 'AWAITING_MERCHANT_APPROVAL', version = version + 1
			WHERE id = $1 AND status NOT IN ('RESOLVED', 'CLOSED')`, caseID)
		var approvalID string
		_ = h.DB.QueryRow(ctx, `
			INSERT INTO approvals
				(case_id, policy_decision_id, action_type, params_sha256, requested_role, decision, expires_at)
			VALUES ($1, $2, 'RECONCILE_MERCHANT_ORDER', $3, 'MERCHANT_ADMIN', 'PENDING', now() + interval '24 hours')
			RETURNING id::text`,
			caseID, decisionID, paramsSHA).Scan(&approvalID)
		_, _ = h.DB.Exec(ctx, `
			INSERT INTO audit_events (case_id, actor_type, actor_id, event_type, visibility, data, event_hash)
			VALUES ($1, 'MERCHANT_USER', $2, 'APPROVAL_REQUESTED', 'MERCHANT', $3, $4)`,
			caseID, claims.Sub,
			fmt.Sprintf(`{"action":"RECONCILE_MERCHANT_ORDER","message":"Reconciliation awaits merchant approval"}`),
			sha256Hex([]byte("APPROVAL_REQUESTED:"+approvalID)))
		writeJSON(w, http.StatusOK, actionResponse{
			Decision: decisionOut,
			Approval: &approvalInfo{ID: approvalID, Decision: "PENDING", ParamsSHA256: paramsSHA},
			Case:     caseState{Status: "AWAITING_MERCHANT_APPROVAL", Version: state.Version + 1},
		})
		return

	case "WAIT":
		writeJSON(w, http.StatusOK, actionResponse{
			Decision: decisionOut,
			Case:     caseState{Status: state.Status, Version: state.Version},
		})
		return

	case "BLOCK":
		writeJSON(w, http.StatusOK, actionResponse{
			Decision: decisionOut,
			Case:     caseState{Status: state.Status, Version: state.Version},
		})
		return

	default:
		writeProblem(w, http.StatusInternalServerError, "Unknown policy outcome", "The policy engine returned an unrecognized decision.")
		return
	}
}

func findingsFromPolicy(in []policy.Finding) []Finding {
	out := make([]Finding, 0, len(in))
	for _, f := range in {
		out = append(out, Finding{
			RuleID:  f.RuleID,
			Code:    f.Code,
			Message: f.Message,
			Passed:  f.Outcome == "ALLOW" && f.Gate != "",
		})
	}
	return out
}

func (h *Handlers) requestRefund(w http.ResponseWriter, r *http.Request, caseID string, req actionRequest) {
	claims := claimsFrom(r)
	state, err := h.loadActionState(r.Context(), caseID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "Case not found", "This case does not exist.")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not load case", "The case state could not be read.")
		return
	}

	amount := req.Params.AmountMinor
	if amount == 0 {
		amount = state.AmountMinor
	}
	currency := req.Params.Currency
	if currency == "" {
		currency = state.Currency
	}
	params := paramsJSON(amount, currency)
	paramsSHA := sha256Hex(params)

	findings := []Finding{
		{RuleID: "PP-301", Code: "APPROVAL_REQUIRED", Message: "A refund always requires a merchant admin approval", Passed: true},
		{RuleID: "PP-302", Code: "GATE_PASSED", Message: fmt.Sprintf("Refund of %d %s matches the verified payment", amount, currency), Passed: true},
	}
	findingsJSON, _ := json.Marshal(findings)
	policyRevision := h.policyRevision(r.Context())

	var decisionID string
	var decisionAt time.Time
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO policy_decisions
			(case_id, evaluation_id, action_type, params_sha256, input, input_sha256,
			 outcome, findings, policy_revision, requested_by)
		VALUES ($1, gen_random_uuid(), 'REQUEST_REFUND', $2, $3, $4,
		        $5, $6, $7, $8)
		RETURNING id::text, created_at`,
		caseID, paramsSHA,
		fmt.Sprintf(`{"schema_version":"1.0","request":{"action":{"type":"REQUEST_REFUND","params":%s}}}`, params),
		sha256Hex(params), string(findingsJSON), policyRevision, claims.Sub).Scan(&decisionID, &decisionAt)
	if err != nil {
		h.Log.Error("insert refund decision", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Policy record failed", "The approval request could not be recorded.")
		return
	}

	var approvalID string
	err = h.DB.QueryRow(r.Context(), `
		INSERT INTO approvals
			(case_id, policy_decision_id, action_type, params_sha256, requested_role, decision, expires_at)
		VALUES ($1, $2, 'REQUEST_REFUND', $3, 'MERCHANT_ADMIN', 'PENDING', now() + interval '24 hours')
		RETURNING id::text`, caseID, decisionID, paramsSHA).Scan(&approvalID)
	if err != nil {
		h.Log.Error("insert approval", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Approval failed", "The approval request could not be created.")
		return
	}

	var newVersion int
	_ = h.DB.QueryRow(r.Context(), `
		UPDATE exception_cases
		SET status = 'AWAITING_MERCHANT_APPROVAL', version = version + 1
		WHERE id = $1 AND status NOT IN ('RESOLVED', 'CLOSED')
		RETURNING version`, caseID).Scan(&newVersion)

	_, _ = h.DB.Exec(r.Context(), `
		INSERT INTO audit_events (case_id, actor_type, actor_id, event_type, visibility, data, event_hash)
		VALUES ($1, 'MERCHANT_USER', $2, 'APPROVAL_REQUESTED', 'MERCHANT', $3, $4)`,
		caseID, claims.Sub,
		fmt.Sprintf(`{"action":"REQUEST_REFUND","approval_id":%q,"message":"Refund request created and waiting for approval"}`, approvalID),
		sha256Hex([]byte("APPROVAL_REQUESTED:"+approvalID)))

	writeJSON(w, http.StatusOK, actionResponse{
		Decision: &Decision{ID: decisionID, ActionType: "REQUEST_REFUND", Outcome: "REQUIRE_APPROVAL", PolicyRevision: policyRevision, Findings: findings, CreatedAt: decisionAt},
		Approval: &approvalInfo{ID: approvalID, Decision: "PENDING", ParamsSHA256: paramsSHA},
		Case:     caseState{Status: "AWAITING_MERCHANT_APPROVAL", Version: newVersion},
	})
}

func (h *Handlers) DecideApproval(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	if !isMerchant(claims) {
		writeProblem(w, http.StatusForbidden, "Merchant action required", "Only a merchant team member can decide an approval.")
		return
	}
	caseID, ok := uuidText(r.PathValue("id"))
	approvalID, okApproval := uuidText(r.PathValue("approvalID"))
	if !ok || !okApproval {
		writeProblem(w, http.StatusNotFound, "Approval not found", "That approval id is not valid.")
		return
	}
	_, _, party, err := h.partyFor(r.Context(), caseID, claims)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !party) {
		writeProblem(w, http.StatusNotFound, "Case not found", "This case is not shared with you.")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not load case", "The case check failed.")
		return
	}

	var req struct {
		Decision string `json:"decision"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid decision", "Expected APPROVED or REJECTED.")
		return
	}
	decision := strings.ToUpper(strings.TrimSpace(req.Decision))
	if decision != "APPROVED" && decision != "REJECTED" {
		writeProblem(w, http.StatusBadRequest, "Invalid decision", "Expected APPROVED or REJECTED.")
		return
	}

	var (
		currentDecision string
		expiresAt       time.Time
	)
	err = h.DB.QueryRow(r.Context(), `
		SELECT decision, expires_at FROM approvals
		WHERE id = $1 AND case_id = $2`, approvalID, caseID).Scan(&currentDecision, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "Approval not found", "That approval does not belong to this case.")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not load approval", "The approval could not be read.")
		return
	}
	if currentDecision != "PENDING" {
		writeProblem(w, http.StatusConflict, "Approval already decided", "This approval has already been decided.")
		return
	}
	if time.Now().After(expiresAt) {
		writeProblem(w, http.StatusConflict, "Approval expired", "This approval has expired; request a new one.")
		return
	}

	_, err = h.DB.Exec(r.Context(), `
		UPDATE approvals
		SET decision = $1, approver_id = $2, decided_at = now()
		WHERE id = $3`, decision, claims.Sub, approvalID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not save decision", "The approval decision could not be stored.")
		return
	}

	if decision == "APPROVED" {
		_, _ = h.DB.Exec(r.Context(), `
			UPDATE exception_cases
			SET status = 'RESOLVED', resolution_code = 'REFUND_APPROVED', closed_at = now(), version = version + 1
			WHERE id = $1`, caseID)
		_, _ = h.DB.Exec(r.Context(), `
			INSERT INTO audit_events (case_id, actor_type, actor_id, event_type, visibility, data, event_hash)
			VALUES ($1, 'MERCHANT_USER', $2, 'REFUND_APPROVED', 'CUSTOMER', $3, $4)`,
			caseID, claims.Sub,
			`{"message":"A merchant admin approved the refund request"}`,
			sha256Hex([]byte("REFUND_APPROVED:"+approvalID)))
	} else {
		_, _ = h.DB.Exec(r.Context(), `
			UPDATE exception_cases
			SET status = 'INVESTIGATING', version = version + 1
			WHERE id = $1`, caseID)
		_, _ = h.DB.Exec(r.Context(), `
			INSERT INTO audit_events (case_id, actor_type, actor_id, event_type, visibility, data, event_hash)
			VALUES ($1, 'MERCHANT_USER', $2, 'APPROVAL_REJECTED', 'MERCHANT', $3, $4)`,
			caseID, claims.Sub,
			`{"message":"The refund request was rejected and the case stays open"}`,
			sha256Hex([]byte("APPROVAL_REJECTED:"+approvalID)))
	}

	detail, found, err := h.buildDetail(r.Context(), caseID, claims)
	if err != nil || !found {
		writeProblem(w, http.StatusInternalServerError, "Could not load case", "The decision was saved but the case could not be reloaded.")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}
