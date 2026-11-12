package webapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"fractal/api/internal/assistant"
	"fractal/api/internal/auth"
)

// merchantScope resolves the merchant a request is acting on. Every assistant
// query is scoped by this id, so one merchant can never see another's cases.
func (h *Handlers) merchantScope(claims *auth.Claims, r *http.Request) (string, bool) {
	if claims == nil {
		return "", false
	}
	if claims.Kind == "PLATFORM_ADMIN" {
		// A platform admin must name the merchant it acts for.
		if id := strings.TrimSpace(r.URL.Query().Get("merchant_id")); id != "" {
			if _, ok := uuidText(id); ok {
				return id, true
			}
		}
		return "", false
	}
	if claims.MerchantID == "" {
		return "", false
	}
	return claims.MerchantID, true
}

// requireMerchant returns the merchant scope for assistant routes.
func (h *Handlers) requireMerchant(w http.ResponseWriter, r *http.Request) (string, *auth.Claims, bool) {
	return h.requireMerchantScoped(w, r, nil)
}

// requireMerchantScoped resolves the merchant scope for a route that points at
// a record. A platform admin may omit the merchant query parameter when the
// record itself already belongs to exactly one merchant: the scope is then
// taken from that record, never guessed.
func (h *Handlers) requireMerchantScoped(w http.ResponseWriter, r *http.Request, owner func(context.Context) (string, error)) (string, *auth.Claims, bool) {
	claims := claimsFrom(r)
	if claims == nil {
		writeProblem(w, http.StatusUnauthorized, "Unauthenticated", "Missing session.")
		return "", nil, false
	}
	if !isMerchant(claims) {
		writeProblem(w, http.StatusForbidden, "Merchant action required",
			"Only a merchant team member can use the case assistant.")
		return "", nil, false
	}
	merchantID, ok := h.merchantScope(claims, r)
	if !ok && claims.Kind == "PLATFORM_ADMIN" && owner != nil {
		resolved, err := owner(r.Context())
		if err == nil && resolved != "" {
			merchantID, ok = resolved, true
		}
	}
	if !ok {
		writeProblem(w, http.StatusForbidden, "Merchant scope required",
			"Choose the merchant this request is for.")
		return "", nil, false
	}
	return merchantID, claims, true
}

// incidentMerchant resolves the merchant that owns an incident.
func (h *Handlers) incidentMerchant(ctx context.Context, incidentID string) (string, error) {
	var merchantID string
	err := h.DB.QueryRow(ctx, `SELECT merchant_id::text FROM incidents WHERE id = $1`, incidentID).Scan(&merchantID)
	return merchantID, err
}

// draftMerchant resolves the merchant that owns a drafted reply.
func (h *Handlers) draftMerchant(ctx context.Context, draftID string) (string, error) {
	var merchantID string
	err := h.DB.QueryRow(ctx, `
		SELECT c.merchant_id::text FROM message_drafts d
		JOIN exception_cases c ON c.id = d.case_id WHERE d.id = $1`, draftID).Scan(&merchantID)
	return merchantID, err
}

// ListIncidents returns the merchant's triage inbox.
func (h *Handlers) ListIncidents(w http.ResponseWriter, r *http.Request) {
	merchantID, _, ok := h.requireMerchant(w, r)
	if !ok {
		return
	}
	if h.Assistant == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Assistant unavailable", "The case assistant is not enabled.")
		return
	}
	incidents, err := h.Assistant.ListIncidents(r.Context(), merchantID, false)
	if err != nil {
		h.Log.Error("list incidents", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not load the inbox", "The incident list could not be read.")
		return
	}
	cases, err := h.Assistant.ListTriageCases(r.Context(), merchantID)
	if err != nil {
		h.Log.Error("list triage cases", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not load the inbox", "The case list could not be read.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"incidents": incidents,
		"cases":     cases,
		"generated_at": time.Now().UTC(),
	})
}

// GetIncident returns one incident with its cases and drafts.
func (h *Handlers) GetIncident(w http.ResponseWriter, r *http.Request) {
	incidentID, valid := uuidText(r.PathValue("id"))
	if !valid {
		writeProblem(w, http.StatusNotFound, "Incident not found", "That incident id is not valid.")
		return
	}
	merchantID, _, ok := h.requireMerchantScoped(w, r, func(ctx context.Context) (string, error) {
		return h.incidentMerchant(ctx, incidentID)
	})
	if !ok {
		return
	}
	if h.Assistant == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Assistant unavailable", "The case assistant is not enabled.")
		return
	}
	incident, err := h.Assistant.GetIncident(r.Context(), merchantID, incidentID)
	if errors.Is(err, assistant.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "Incident not found", "This incident does not exist or is not shared with you.")
		return
	}
	if err != nil {
		h.Log.Error("get incident", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not load the incident", "The incident could not be read.")
		return
	}
	drafts, err := h.Assistant.ListDrafts(r.Context(), merchantID, incidentID, "")
	if err != nil {
		h.Log.Error("list incident drafts", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not load the incident", "The drafts could not be read.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"incident": incident,
		"drafts":   drafts,
	})
}

// GroupCase runs incident grouping for one case.
func (h *Handlers) GroupCase(w http.ResponseWriter, r *http.Request) {
	_, _, ok := h.requireMerchant(w, r)
	if !ok {
		return
	}
	if h.Assistant == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Assistant unavailable", "The case assistant is not enabled.")
		return
	}
	caseID, valid := uuidText(r.PathValue("id"))
	if !valid {
		writeProblem(w, http.StatusNotFound, "Case not found", "That case id is not valid.")
		return
	}
	if _, _, party, err := h.partyFor(r.Context(), caseID, claimsFrom(r)); err != nil || !party {
		writeProblem(w, http.StatusNotFound, "Case not found", "This case is not shared with you.")
		return
	}
	incident, err := h.Assistant.GroupCase(r.Context(), caseID)
	if err != nil {
		h.Log.Error("group case", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not group the case", "The case could not be grouped.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incident": incident})
}

// RegroupMerchant re-runs grouping for every open case of the merchant.
func (h *Handlers) RegroupMerchant(w http.ResponseWriter, r *http.Request) {
	merchantID, _, ok := h.requireMerchant(w, r)
	if !ok {
		return
	}
	if h.Assistant == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Assistant unavailable", "The case assistant is not enabled.")
		return
	}
	if err := h.Assistant.RegroupMerchant(r.Context(), merchantID); err != nil {
		h.Log.Error("regroup merchant", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not regroup the cases", "Grouping did not finish.")
		return
	}
	incidents, err := h.Assistant.ListIncidents(r.Context(), merchantID, false)
	if err != nil {
		h.Log.Error("list incidents after regroup", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not regroup the cases", "The incident list could not be read.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incidents": incidents})
}

// DraftIncident drafts a reply for every linked case of an incident.
func (h *Handlers) DraftIncident(w http.ResponseWriter, r *http.Request) {
	incidentID, valid := uuidText(r.PathValue("id"))
	if !valid {
		writeProblem(w, http.StatusNotFound, "Incident not found", "That incident id is not valid.")
		return
	}
	merchantID, claims, ok := h.requireMerchantScoped(w, r, func(ctx context.Context) (string, error) {
		return h.incidentMerchant(ctx, incidentID)
	})
	if !ok {
		return
	}
	if h.Assistant == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Assistant unavailable", "The case assistant is not enabled.")
		return
	}
	drafts, err := h.Assistant.DraftIncident(r.Context(), merchantID, incidentID, claims.Sub)
	if err != nil {
		h.Log.Error("draft incident", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not draft the replies",
			"The replies could not be drafted from the verified facts.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"drafts": drafts})
}

// DraftCase drafts a reply for one case. It is used from the case page when
// a case is not part of a group.
func (h *Handlers) DraftCase(w http.ResponseWriter, r *http.Request) {
	merchantID, claims, ok := h.requireMerchant(w, r)
	if !ok {
		return
	}
	if h.Assistant == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Assistant unavailable", "The case assistant is not enabled.")
		return
	}
	caseID, valid := uuidText(r.PathValue("id"))
	if !valid {
		writeProblem(w, http.StatusNotFound, "Case not found", "That case id is not valid.")
		return
	}
	if _, _, party, err := h.partyFor(r.Context(), caseID, claims); err != nil || !party {
		writeProblem(w, http.StatusNotFound, "Case not found", "This case is not shared with you.")
		return
	}
	draft, err := h.Assistant.DraftCase(r.Context(), assistant.DraftRequest{
		MerchantID: merchantID,
		CaseID:     caseID,
		ActorID:    claims.Sub,
	})
	var guard *assistant.GuardrailError
	if errors.As(err, &guard) {
		writeProblem(w, http.StatusUnprocessableEntity, "Reply not drafted", guard.Error())
		return
	}
	if err != nil {
		h.Log.Error("draft case", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not draft the reply",
			"The reply could not be drafted from the verified facts.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"draft": draft})
}

// ApproveIncidentDrafts approves every drafted reply of an incident.
func (h *Handlers) ApproveIncidentDrafts(w http.ResponseWriter, r *http.Request) {
	incidentID, valid := uuidText(r.PathValue("id"))
	if !valid {
		writeProblem(w, http.StatusNotFound, "Incident not found", "That incident id is not valid.")
		return
	}
	merchantID, claims, ok := h.requireMerchantScoped(w, r, func(ctx context.Context) (string, error) {
		return h.incidentMerchant(ctx, incidentID)
	})
	if !ok {
		return
	}
	if h.Assistant == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Assistant unavailable", "The case assistant is not enabled.")
		return
	}
	drafts, err := h.Assistant.ApproveIncidentDrafts(r.Context(), merchantID, incidentID, claims.Sub)
	if err != nil {
		h.Log.Error("approve incident drafts", "err", err)
		writeProblem(w, http.StatusUnprocessableEntity, "Some replies were not approved", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"drafts": drafts})
}

// SendIncidentDrafts sends every approved reply of an incident.
func (h *Handlers) SendIncidentDrafts(w http.ResponseWriter, r *http.Request) {
	incidentID, valid := uuidText(r.PathValue("id"))
	if !valid {
		writeProblem(w, http.StatusNotFound, "Incident not found", "That incident id is not valid.")
		return
	}
	merchantID, claims, ok := h.requireMerchantScoped(w, r, func(ctx context.Context) (string, error) {
		return h.incidentMerchant(ctx, incidentID)
	})
	if !ok {
		return
	}
	if h.Assistant == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Assistant unavailable", "The case assistant is not enabled.")
		return
	}
	results, err := h.Assistant.SendIncidentDrafts(r.Context(), merchantID, incidentID, claims.Sub)
	if err != nil {
		h.Log.Error("send incident drafts", "err", err)
		writeProblem(w, http.StatusUnprocessableEntity, "Some replies were not sent", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": results})
}

// ApproveDraft approves one drafted reply.
func (h *Handlers) ApproveDraft(w http.ResponseWriter, r *http.Request) {
	draftID, valid := uuidText(r.PathValue("id"))
	if !valid {
		writeProblem(w, http.StatusNotFound, "Draft not found", "That draft id is not valid.")
		return
	}
	merchantID, claims, ok := h.requireMerchantScoped(w, r, func(ctx context.Context) (string, error) {
		return h.draftMerchant(ctx, draftID)
	})
	if !ok {
		return
	}
	if h.Assistant == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Assistant unavailable", "The case assistant is not enabled.")
		return
	}
	draft, err := h.Assistant.ApproveDraft(r.Context(), merchantID, draftID, claims.Sub)
	if errors.Is(err, assistant.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "Draft not found", "That draft does not exist for this merchant.")
		return
	}
	if err != nil {
		h.Log.Error("approve draft", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not approve the reply", "The approval could not be recorded.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"draft": draft})
}

// UpdateDraft edits a drafted reply.
func (h *Handlers) UpdateDraft(w http.ResponseWriter, r *http.Request) {
	draftID, valid := uuidText(r.PathValue("id"))
	if !valid {
		writeProblem(w, http.StatusNotFound, "Draft not found", "That draft id is not valid.")
		return
	}
	merchantID, _, ok := h.requireMerchantScoped(w, r, func(ctx context.Context) (string, error) {
		return h.draftMerchant(ctx, draftID)
	})
	if !ok {
		return
	}
	if h.Assistant == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Assistant unavailable", "The case assistant is not enabled.")
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid draft", "Expected the draft body as JSON.")
		return
	}
	draft, err := h.Assistant.UpdateDraft(r.Context(), merchantID, draftID, req.Body)
	if errors.Is(err, assistant.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "Draft not found", "That draft does not exist for this merchant.")
		return
	}
	var guard *assistant.GuardrailError
	if errors.As(err, &guard) {
		writeProblem(w, http.StatusUnprocessableEntity, "Draft rejected", guard.Error())
		return
	}
	if err != nil {
		h.Log.Error("update draft", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not save the reply", "The draft could not be saved.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"draft": draft})
}

// SendDraft sends one approved reply.
func (h *Handlers) SendDraft(w http.ResponseWriter, r *http.Request) {
	draftID, valid := uuidText(r.PathValue("id"))
	if !valid {
		writeProblem(w, http.StatusNotFound, "Draft not found", "That draft id is not valid.")
		return
	}
	merchantID, claims, ok := h.requireMerchantScoped(w, r, func(ctx context.Context) (string, error) {
		return h.draftMerchant(ctx, draftID)
	})
	if !ok {
		return
	}
	if h.Assistant == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Assistant unavailable", "The case assistant is not enabled.")
		return
	}
	result, err := h.Assistant.SendDraft(r.Context(), merchantID, draftID, claims.Sub)
	if errors.Is(err, assistant.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "Draft not found", "That draft does not exist for this merchant.")
		return
	}
	var guard *assistant.GuardrailError
	if errors.As(err, &guard) {
		writeProblem(w, http.StatusUnprocessableEntity, "Reply not sent", guard.Error())
		return
	}
	if err != nil {
		h.Log.Error("send draft", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not send the reply", "The reply could not be sent.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": result})
}

// DiscardDraft drops a drafted reply.
func (h *Handlers) DiscardDraft(w http.ResponseWriter, r *http.Request) {
	draftID, valid := uuidText(r.PathValue("id"))
	if !valid {
		writeProblem(w, http.StatusNotFound, "Draft not found", "That draft id is not valid.")
		return
	}
	merchantID, _, ok := h.requireMerchantScoped(w, r, func(ctx context.Context) (string, error) {
		return h.draftMerchant(ctx, draftID)
	})
	if !ok {
		return
	}
	if h.Assistant == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Assistant unavailable", "The case assistant is not enabled.")
		return
	}
	if err := h.Assistant.DiscardDraft(r.Context(), merchantID, draftID); err != nil {
		writeProblem(w, http.StatusNotFound, "Draft not found", "That draft does not exist for this merchant.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "DISCARDED"})
}

// AddIncidentCase links a case to an incident at the merchant's request.
func (h *Handlers) AddIncidentCase(w http.ResponseWriter, r *http.Request) {
	incidentID, valid := uuidText(r.PathValue("id"))
	caseID, validCase := uuidText(r.PathValue("caseID"))
	if !valid || !validCase {
		writeProblem(w, http.StatusNotFound, "Case not found", "That case id is not valid.")
		return
	}
	merchantID, _, ok := h.requireMerchantScoped(w, r, func(ctx context.Context) (string, error) {
		return h.incidentMerchant(ctx, incidentID)
	})
	if !ok {
		return
	}
	if h.Assistant == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Assistant unavailable", "The case assistant is not enabled.")
		return
	}
	if err := h.Assistant.AddCase(r.Context(), merchantID, incidentID, caseID); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "Case not added", err.Error())
		return
	}
	incident, err := h.Assistant.GetIncident(r.Context(), merchantID, incidentID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not load the incident", "The incident could not be read.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incident": incident})
}

// RemoveIncidentCase unlinks a case from an incident at the merchant's request.
func (h *Handlers) RemoveIncidentCase(w http.ResponseWriter, r *http.Request) {
	incidentID, valid := uuidText(r.PathValue("id"))
	caseID, validCase := uuidText(r.PathValue("caseID"))
	if !valid || !validCase {
		writeProblem(w, http.StatusNotFound, "Case not found", "That case id is not valid.")
		return
	}
	merchantID, _, ok := h.requireMerchantScoped(w, r, func(ctx context.Context) (string, error) {
		return h.incidentMerchant(ctx, incidentID)
	})
	if !ok {
		return
	}
	if h.Assistant == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Assistant unavailable", "The case assistant is not enabled.")
		return
	}
	if err := h.Assistant.RemoveCase(r.Context(), merchantID, incidentID, caseID); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "Case not removed", err.Error())
		return
	}
	incident, err := h.Assistant.GetIncident(r.Context(), merchantID, incidentID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not load the incident", "The incident could not be read.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"incident": incident})
}

// fanOutResult is one case's outcome inside an incident fan-out.
type fanOutResult struct {
	CaseID     string `json:"case_id"`
	CaseNumber string `json:"case_number"`
	Status     string `json:"status"`
	Outcome    string `json:"outcome"`
	Detail     string `json:"detail"`
}

// FanOutIncident reconciles every linked case of an incident.
//
// The assistant proposes one action per linked case; each case then goes
// through the same per-case path, so PolicyGuard evaluates it on its own.
// An amount mismatch in one case blocks only that case.
func (h *Handlers) FanOutIncident(w http.ResponseWriter, r *http.Request) {
	incidentID, valid := uuidText(r.PathValue("id"))
	if !valid {
		writeProblem(w, http.StatusNotFound, "Incident not found", "That incident id is not valid.")
		return
	}
	merchantID, claims, ok := h.requireMerchantScoped(w, r, func(ctx context.Context) (string, error) {
		return h.incidentMerchant(ctx, incidentID)
	})
	if !ok {
		return
	}
	if h.Assistant == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Assistant unavailable", "The case assistant is not enabled.")
		return
	}
	incident, err := h.Assistant.GetIncident(r.Context(), merchantID, incidentID)
	if errors.Is(err, assistant.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "Incident not found", "This incident does not exist or is not shared with you.")
		return
	}
	if err != nil {
		h.Log.Error("fan out incident", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not reconcile the cases", "The incident could not be read.")
		return
	}

	results := []fanOutResult{}
	for _, c := range incident.Cases {
		if c.Status == "RESOLVED" || c.Status == "CLOSED" {
			results = append(results, fanOutResult{
				CaseID: c.CaseID, CaseNumber: c.CaseNumber, Status: "SKIPPED",
				Outcome: "ALREADY_RESOLVED", Detail: "The case is already closed.",
			})
			continue
		}
		var req actionRequest
		req.ActionType = "RECONCILE_MERCHANT_ORDER"
		result := h.reconcileCase(r.Context(), claims, c.CaseID, req)
		outcome := result.Outcome()
		detail := ""
		status := "COMPLETED"
		if result.problem {
			status = "FAILED"
			detail = result.detail
		}
		if outcome == "" && !result.problem {
			outcome = "COMPLETED"
		}
		results = append(results, fanOutResult{
			CaseID: c.CaseID, CaseNumber: c.CaseNumber, Status: status,
			Outcome: outcome, Detail: detail,
		})
	}

	_ = h.Assistant.RefreshCase(r.Context(), incidentID)
	writeJSON(w, http.StatusOK, map[string]any{
		"incident_id": incidentID,
		"results":     results,
	})
}

// ensure the assistant package import is used even when guards change.
var _ = pgx.ErrNoRows
