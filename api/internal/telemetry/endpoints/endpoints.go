package endpoints

import (
	"fmt"
	"net/http"
)

// LogsByCase handles GET /internal/v1/telemetry/logs?case_id=&window=.
// Read-only agent tool: searches Elastic logs scoped to a case.
func LogsByCase(w http.ResponseWriter, r *http.Request) {
	caseID := r.URL.Query().Get("case_id")
	window := r.URL.Query().Get("window")
	if caseID == "" {
		http.Error(w, `{"error":"case_id is required"}`, http.StatusBadRequest)
		return
	}
	// TODO: real Elasticsearch query: index=fractal-logs, filter case_id, window.
	// Returns log lines for the agent to correlate handler errors / trace gaps.
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"case_id":"%s","window":"%s","hits":[],"note":"elastic client not wired in this step"}`,
		caseID, window)
}

// TraceByID handles GET /internal/v1/telemetry/traces/{trace_id}.
// Read-only agent tool: fetches a single trace span tree.
func TraceByID(w http.ResponseWriter, r *http.Request) {
	traceID := r.PathValue("trace_id")
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"trace_id":"%s","spans":[],"note":"elastic trace lookup not wired in this step"}`, traceID)
}

// HandlerErrorRate handles GET /internal/v1/telemetry/handler-error-rate?merchant_id=&window=.
// Read-only agent tool: aggregate metric for per-merchant handler failure rate.
func HandlerErrorRate(w http.ResponseWriter, r *http.Request) {
	merchantID := r.URL.Query().Get("merchant_id")
	window := r.URL.Query().Get("window")
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"merchant_id":"%s","window":"%s","error_rate":0.0,"count":0,"note":"metrics not wired in this step"}`, merchantID, window)
}

