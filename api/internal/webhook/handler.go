// Package webhook receives, verifies, deduplicates and stores provider webhooks.
//
// Receiver sequence (see Spec C6):
//
//	raw body -> required headers -> account lookup -> verify with provider
//	-> parse -> INSERT ... ON CONFLICT DO NOTHING -> 200 -> async processing
//
// Status code rules (PayPal retries any non-2xx for up to 3 days):
//
//	200  verified event stored (or already stored: duplicate)
//	400  not authentic or malformed (security event recorded, nothing stored)
//	404  unknown provider account
//	413  body too large
//	500  could not persist the event (provider will retry)
//	503  could not decide authenticity right now (provider will retry)
package webhook

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"

	"fractal/api/internal/provider"
)

const maxBodyBytes = 1 << 20 // 1 MiB

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ErrAccountNotFound means the provider account is unknown, inactive or has no webhook ID.
var ErrAccountNotFound = errors.New("provider account not found")

// Event holds the minimal fields FRACTAL needs from a provider event.
type Event struct {
	ID        string `json:"id"`
	EventType string `json:"event_type"`
	Resource  struct {
		ID string `json:"id"`
	} `json:"resource"`
}

type SecurityEvent struct {
	Type          string
	Provider      string
	SourceIP      string
	PayloadSHA256 string
	Detail        map[string]any
}

type AccountResolver interface {
	// WebhookID returns the provider-issued webhook ID registered for the account.
	WebhookID(ctx context.Context, providerAccountID string) (string, error)
}

type Store interface {
	// InsertVerified stores a verified event. inserted is false for a duplicate.
	InsertVerified(ctx context.Context, providerAccountID string, ev Event, raw []byte, sha256hex string) (inserted bool, err error)
	RecordSecurityEvent(ctx context.Context, se SecurityEvent) error
}

type Handler struct {
	Provider string // "PAYPAL"
	Accounts AccountResolver
	Verifier provider.WebhookVerifier
	Store    Store
	Wake     func() // optional: nudges the worker after a new event is stored
	Log      *slog.Logger
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := h.logger()
	accountID := r.PathValue("provider_account_id")

	// 1. Raw body first. The exact bytes are what the provider signed.
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeJSON(w, http.StatusRequestEntityTooLarge, "payload_too_large")
			return
		}
		writeJSON(w, http.StatusBadRequest, "unreadable_body")
		return
	}
	sum := sha256.Sum256(raw)
	sha := hex.EncodeToString(sum[:])

	reject := func(eventType string, detail map[string]any) {
		h.security(ctx, r, eventType, sha, detail)
		writeJSON(w, http.StatusBadRequest, "rejected")
	}

	// 2. Required signature headers.
	hdr := provider.WebhookHeaders{
		TransmissionID:   r.Header.Get("PAYPAL-TRANSMISSION-ID"),
		TransmissionTime: r.Header.Get("PAYPAL-TRANSMISSION-TIME"),
		TransmissionSig:  r.Header.Get("PAYPAL-TRANSMISSION-SIG"),
		CertURL:          r.Header.Get("PAYPAL-CERT-URL"),
		AuthAlgo:         r.Header.Get("PAYPAL-AUTH-ALGO"),
	}
	if !hdr.Complete() {
		reject("WEBHOOK_HEADERS_MISSING", map[string]any{"account": accountID})
		return
	}

	// 3. Which webhook ID should this delivery be verified against?
	if !uuidRe.MatchString(accountID) {
		writeJSON(w, http.StatusNotFound, "unknown_account")
		return
	}
	webhookID, err := h.Accounts.WebhookID(ctx, accountID)
	if errors.Is(err, ErrAccountNotFound) {
		writeJSON(w, http.StatusNotFound, "unknown_account")
		return
	}
	if err != nil {
		log.Error("account lookup failed", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, "retry_later")
		return
	}

	// 4. Verify with the provider. Undecided means retry later, never store.
	ok, err := h.Verifier.VerifyWebhook(ctx, webhookID, hdr, raw)
	if err != nil {
		log.Error("webhook verification undecided", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, "retry_later")
		return
	}
	if !ok {
		reject("WEBHOOK_SIGNATURE_INVALID", map[string]any{"account": accountID, "transmission_id": hdr.TransmissionID})
		return
	}

	// 5. Parse the minimal fields from the now-trusted event.
	var ev Event
	if err := json.Unmarshal(raw, &ev); err != nil || ev.ID == "" || ev.EventType == "" {
		reject("WEBHOOK_MALFORMED", map[string]any{"account": accountID})
		return
	}

	// 6. Persist. The unique key (provider, provider_event_id) deduplicates.
	inserted, err := h.Store.InsertVerified(ctx, accountID, ev, raw, sha)
	if err != nil {
		log.Error("webhook persist failed", "err", err, "event_id", ev.ID)
		writeJSON(w, http.StatusInternalServerError, "retry_later")
		return
	}
	if !inserted {
		log.Info("duplicate webhook ignored", "event_id", ev.ID)
		writeJSON(w, http.StatusOK, "duplicate")
		return
	}

	// 7. Acknowledge now. Processing happens asynchronously from the stored row.
	if h.Wake != nil {
		h.Wake()
	}
	log.Info("webhook accepted", "event_id", ev.ID, "type", ev.EventType)
	writeJSON(w, http.StatusOK, "accepted")
}

func (h *Handler) security(ctx context.Context, r *http.Request, eventType, sha string, detail map[string]any) {
	// RemoteAddr is the tunnel or proxy when running locally. Only trust
	// X-Forwarded-For if you control the proxy in front of this service.
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	se := SecurityEvent{Type: eventType, Provider: h.Provider, SourceIP: ip, PayloadSHA256: sha, Detail: detail}
	if err := h.Store.RecordSecurityEvent(ctx, se); err != nil {
		h.logger().Error("security event not recorded", "err", err, "type", eventType)
	}
}

func (h *Handler) logger() *slog.Logger {
	if h.Log != nil {
		return h.Log
	}
	return slog.Default()
}

func writeJSON(w http.ResponseWriter, status int, result string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"status": result})
}
