package webhook

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fractal/api/internal/provider"
)

type Receiver struct {
	provider   provider.Provider
	db         *pgxpool.Pool
	webhookID  string
	logger     *slog.Logger
}

func NewReceiver(p provider.Provider, db *pgxpool.Pool, webhookID string, logger *slog.Logger) *Receiver {
	return &Receiver{provider: p, db: db, webhookID: webhookID, logger: logger}
}

func (r *Receiver) Handle(w http.ResponseWriter, req *http.Request) {
	// 1. Read raw body before any JSON parsing.
	rawBody, err := io.ReadAll(req.Body)
	if err != nil {
		r.logger.Error("read body", "err", err)
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}

	headers := map[string]string{
		"Paypal-Transmission-Id": req.Header.Get("Paypal-Transmission-Id"),
		"Paypal-Transmission-Time": req.Header.Get("Paypal-Transmission-Time"),
		"Paypal-Cert-Url":        req.Header.Get("Paypal-Cert-Url"),
		"Paypal-Auth-Algo":       req.Header.Get("Paypal-Auth-Algo"),
		"Paypal-Transmission-Sig": req.Header.Get("Paypal-Transmission-Sig"),
		"Paypal-Webhook-Id":      r.webhookID,
	}

	// 2. Verify via PayPal verify-webhook-signature.
	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Second)
	defer cancel()
	verified, err := r.provider.VerifyWebhook(ctx, rawBody, headers)
	if err != nil || !verified {
		r.logSecurityEvent(ctx, "WEBHOOK_SIGNATURE_INVALID", map[string]any{"err": err})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid signature"}`))
		return
	}

	// 3. Parse event id for dedup key.
	var event struct {
		ID        string `json:"id"`
		EventType string `json:"event_type"`
		Resource  struct {
			ID string `json:"id"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(rawBody, &event); err != nil {
		r.logger.Error("parse event", "err", err)
		http.Error(w, `{"error":"bad event body"}`, http.StatusBadRequest)
		return
	}

	// 4. Insert with dedup on (provider, provider_event_id). Replay -> no row -> 200, do nothing.
	var inserted bool
	inserted, err = r.insertWebhook(ctx, event.ID, event.EventType, event.Resource.ID, rawBody)
	if err != nil {
		r.logger.Error("insert webhook", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	if !inserted {
		r.logger.Info("webhook replay ignored", "event_id", event.ID)
	}

	// 5. Respond fast; async worker normalizes evidence.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}

func (r *Receiver) insertWebhook(ctx context.Context, eventID, eventType, resourceID string, payload []byte) (bool, error) {
	sum := sha256.Sum256(payload)
	payloadSHA := hex.EncodeToString(sum[:])
	var id string
	err := r.db.QueryRow(ctx, `
		INSERT INTO webhook_events (provider, provider_event_id, event_type, resource_id, signature_verified, payload, payload_sha256)
		VALUES ('PAYPAL', $1, $2, $3, true, $4, $5)
		ON CONFLICT (provider, provider_event_id) DO NOTHING
		RETURNING id`, eventID, eventType, resourceID, string(payload), payloadSHA).Scan(&id)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (r *Receiver) logSecurityEvent(ctx context.Context, eventType string, detail map[string]any) {
	detailJSON, _ := json.Marshal(detail)
	_, _ = r.db.Exec(ctx, `
		INSERT INTO security_events (event_type, provider, detail)
		VALUES ($1, 'PAYPAL', $2)`, eventType, string(detailJSON))
}

