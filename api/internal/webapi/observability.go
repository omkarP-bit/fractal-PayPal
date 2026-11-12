package webapi

import (
	"database/sql"
	"net/http"
	"os"
	"strings"
	"time"
)

type webhookEventSummary struct {
	EventID           string    `json:"event_id"`
	EventType         string    `json:"event_type"`
	ProviderAccountID string    `json:"provider_account_id"`
	SignatureVerified bool      `json:"signature_verified"`
	ProcessingStatus  string    `json:"processing_status"`
	ReceivedAt        time.Time `json:"received_at"`
}

// ObservabilitySummary is the live data behind the demo page. The mock charts
// that used to live there are gone; this endpoint reports real verified webhook
// traffic for the signed-in merchant, and points at the real Kibana dashboard.
func (h *Handlers) ObservabilitySummary(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	if claims == nil || claims.MerchantID == "" {
		writeProblem(w, http.StatusForbidden, "Merchant account required", "Sign in with a merchant account.")
		return
	}
	if claims.Kind != "MERCHANT_USER" && claims.Kind != "PLATFORM_ADMIN" {
		writeProblem(w, http.StatusForbidden, "Merchant account required", "Sign in with a merchant account.")
		return
	}

	merchantID := claims.MerchantID

	var (
		total, last24h int64
		lastReceived   sql.NullTime
	)
	err := h.DB.QueryRow(r.Context(), `
		SELECT count(*),
		       count(*) FILTER (WHERE we.received_at > now() - interval '24 hours'),
		       max(we.received_at)
		FROM webhook_events we
		JOIN provider_accounts pa ON pa.id = we.provider_account_id
		WHERE pa.merchant_id = $1`, merchantID).
		Scan(&total, &last24h, &lastReceived)
	if err != nil {
		h.Log.Error("load webhook summary", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not load observability", "The webhook summary query failed.")
		return
	}

	account, err := h.providerAccount(r.Context(), merchantID)
	if err != nil {
		h.Log.Error("load observability account", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not load observability", "The provider account query failed.")
		return
	}

	rows, err := h.DB.Query(r.Context(), `
		SELECT we.provider_event_id, we.event_type, COALESCE(we.provider_account_id::text, ''),
		       we.signature_verified, we.processing_status, we.received_at
		FROM webhook_events we
		JOIN provider_accounts pa ON pa.id = we.provider_account_id
		WHERE pa.merchant_id = $1
		ORDER BY we.received_at DESC
		LIMIT 12`, merchantID)
	if err != nil {
		h.Log.Error("load recent webhook events", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not load observability", "The recent events query failed.")
		return
	}
	defer rows.Close()

	recent := make([]webhookEventSummary, 0)
	for rows.Next() {
		var event webhookEventSummary
		if err := rows.Scan(&event.EventID, &event.EventType, &event.ProviderAccountID,
			&event.SignatureVerified, &event.ProcessingStatus, &event.ReceivedAt); err != nil {
			writeProblem(w, http.StatusInternalServerError, "Could not load observability", "A webhook row could not be read.")
			return
		}
		recent = append(recent, event)
	}

	kibanaURL := strings.TrimRight(strings.TrimSpace(h.KibanaURL), "/")
	if kibanaURL == "" {
		kibanaURL = strings.TrimRight(strings.TrimSpace(os.Getenv("KIBANA_URL")), "/")
	}
	if kibanaURL == "" {
		kibanaURL = "http://localhost:5601"
	}

	var lastReceivedValue any
	if lastReceived.Valid {
		lastReceivedValue = lastReceived.Time
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"generated_at":     time.Now().UTC(),
		"merchant_id":      merchantID,
		"kibana_url":       kibanaURL,
		"provider_account": account,
		"webhook_events": map[string]any{
			"total":         total,
			"last_24h":      last24h,
			"last_received": lastReceivedValue,
		},
		"recent_events": recent,
	})
}
