package provider

import (
	"context"
	"time"

	"fractal/api/internal/evidence"
)

// WebhookHeaders are the signature-related headers a provider sends with
// every webhook delivery.
type WebhookHeaders struct {
	TransmissionID   string
	TransmissionTime string
	TransmissionSig  string
	CertURL          string
	AuthAlgo         string
}

func (h WebhookHeaders) Complete() bool {
	return h.TransmissionID != "" && h.TransmissionTime != "" &&
		h.TransmissionSig != "" && h.CertURL != "" && h.AuthAlgo != ""
}

// WebhookVerifier confirms that a webhook really came from the provider.
type WebhookVerifier interface {
	VerifyWebhook(ctx context.Context, webhookID string, h WebhookHeaders, rawBody []byte) (bool, error)
}

// Provider is the payment-provider boundary. PayPal is the first
// implementation. Other providers can be added without touching callers.
type Provider interface {
	WebhookVerifier
	GetTransaction(ctx context.Context, providerOrderID string) (*evidence.Transaction, error)
	Refund(ctx context.Context, captureID string, amountMinor int64, currency string, idempotencyKey string) (string, error)
	ListWebhookEvents(ctx context.Context) ([]WebhookEvent, error)
}

// WebhookEvent is a normalized provider webhook event.
type WebhookEvent struct {
	ID        string
	EventType string
	ResourceID string
}

var _ = time.Time{}
