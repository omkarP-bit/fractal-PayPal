package policy

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

// maxRequestBodyBytes bounds the evaluate request body (1 MiB, same as the
// webhook receiver).
const maxRequestBodyBytes = 1 << 20

// EvaluateRequest is the body of POST /internal/v1/policy/evaluate.
type EvaluateRequest struct {
	Input Input `json:"input"`
	// AnomalyScore is produced by the Phase 11 scoring pipeline
	// (anomaly_scores table / scripts/anomaly CLI). It is advisory: the
	// server-side AnomalyEnabled gate decides whether it ever reaches OPA.
	AnomalyScore *float64 `json:"anomaly_score,omitempty"`
}

// EvaluateResponse is returned with HTTP 200 in every case where the body
// parsed: a policy evaluation always yields a decision, and failures yield
// a fail-closed BLOCK decision (see Client.Evaluate).
type EvaluateResponse struct {
	Decision Decision `json:"decision"`
}

// Handler serves POST /internal/v1/policy/evaluate (internal, same trust
// level as the Phase 10 internal telemetry endpoints).
type Handler struct {
	Client         *Client
	AnomalyEnabled bool
	Log            *slog.Logger
}

func (h *Handler) log() *slog.Logger {
	if h.Log != nil {
		return h.Log
	}
	return slog.Default()
}

// Evaluate runs one guard decision. It never returns 5xx for an OPA
// problem: degradation is expressed as a BLOCK decision (fail closed).
func (h *Handler) Evaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
	if err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	var req EvaluateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.Input.Request.Action.Type == "" {
		http.Error(w, `{"error":"input.request.action.type is required"}`, http.StatusBadRequest)
		return
	}

	// Server-side gate: while disabled the score is always null (Phase 10
	// equivalence), regardless of what the caller sent.
	in := ApplyAnomalyScore(req.Input, h.AnomalyEnabled, req.AnomalyScore)

	decision, err := h.Client.Evaluate(r.Context(), in)
	if err != nil {
		h.log().Warn("policy evaluation failed closed", "err", err,
			"action", req.Input.Request.Action.Type)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(EvaluateResponse{Decision: decision})
}
