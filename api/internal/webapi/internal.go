package webapi

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

type internalEvidence struct {
	ID           string         `json:"id"`
	EvidenceType string         `json:"evidence_type"`
	SourceSystem string         `json:"source_system"`
	SourceRef    string         `json:"source_ref"`
	FetchedAt    time.Time      `json:"fetched_at"`
	Payload      map[string]any `json:"payload"`
}

type internalWebhookEvent struct {
	ID               string         `json:"id"`
	ProviderEventID  string         `json:"provider_event_id"`
	EventType        string         `json:"event_type"`
	ResourceID       string         `json:"resource_id"`
	ProcessingStatus string         `json:"processing_status"`
	ReceivedAt       time.Time      `json:"received_at"`
	Payload          map[string]any `json:"payload"`
}

func internalTokenOK(r *http.Request) bool {
	expected := strings.TrimSpace(os.Getenv("INVESTIGATOR_TOKEN"))
	if expected == "" {
		return false
	}
	got := r.Header.Get("X-Fractal-Internal-Token")
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

// InternalCaseEvidence serves GET /internal/v1/cases/{id}/evidence.
// Read-only tool for the investigator.
func (h *Handlers) InternalCaseEvidence(w http.ResponseWriter, r *http.Request) {
	if !internalTokenOK(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	caseID := strings.TrimSpace(r.PathValue("id"))
	if caseID == "" {
		http.Error(w, `{"error":"case id is required"}`, http.StatusBadRequest)
		return
	}
	evidenceType := strings.TrimSpace(r.URL.Query().Get("type"))

	rows, err := h.DB.Query(r.Context(), `
		SELECT id::text, evidence_type, source_system, source_ref, fetched_at, payload
		FROM case_evidence
		WHERE case_id = $1 AND ($2 = '' OR evidence_type = $2)
		ORDER BY fetched_at ASC`, caseID, evidenceType)
	if err != nil {
		http.Error(w, `{"error":"evidence query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := make([]internalEvidence, 0)
	for rows.Next() {
		var item internalEvidence
		var payload []byte
		if err := rows.Scan(&item.ID, &item.EvidenceType, &item.SourceSystem, &item.SourceRef, &item.FetchedAt, &payload); err != nil {
			http.Error(w, `{"error":"evidence decode failed"}`, http.StatusInternalServerError)
			return
		}
		item.Payload = decodeObject(payload)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, `{"error":"evidence read failed"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, http.StatusOK, map[string]any{"evidence": out})
}

// InternalCaseWebhookEvents serves GET /internal/v1/cases/{id}/webhook-events.
// Read-only tool for the investigator.
func (h *Handlers) InternalCaseWebhookEvents(w http.ResponseWriter, r *http.Request) {
	if !internalTokenOK(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	caseID := strings.TrimSpace(r.PathValue("id"))
	if caseID == "" {
		http.Error(w, `{"error":"case id is required"}`, http.StatusBadRequest)
		return
	}

	rows, err := h.DB.Query(r.Context(), `
		SELECT w.id::text, w.provider_event_id, w.event_type, COALESCE(w.resource_id, ''),
		       w.processing_status, w.received_at, w.payload
		FROM webhook_events w
		WHERE w.provider = 'PAYPAL'
		  AND w.resource_id IN (
			SELECT p.provider_order_id FROM payment_transactions p
			JOIN exception_cases c ON c.payment_transaction_id = p.id
			WHERE c.id = $1
			UNION
			SELECT p.capture_id FROM payment_transactions p
			JOIN exception_cases c ON c.payment_transaction_id = p.id
			WHERE c.id = $1 AND p.capture_id IS NOT NULL
		  )
		ORDER BY w.received_at ASC`, caseID)
	if err != nil {
		http.Error(w, `{"error":"webhook query failed"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := make([]internalWebhookEvent, 0)
	for rows.Next() {
		var item internalWebhookEvent
		var payload []byte
		if err := rows.Scan(&item.ID, &item.ProviderEventID, &item.EventType, &item.ResourceID,
			&item.ProcessingStatus, &item.ReceivedAt, &payload); err != nil {
			http.Error(w, `{"error":"webhook decode failed"}`, http.StatusInternalServerError)
			return
		}
		item.Payload = decodeObject(payload)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, `{"error":"webhook read failed"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"events": out})
}

func decodeObject(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{"raw": string(raw)}
	}
	return out
}
