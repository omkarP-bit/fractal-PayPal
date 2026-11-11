package provider

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"fractal/api/internal/evidence"
)

// PhonePe is a provider implementation for PhonePe (Phase 13 placeholder).
type PhonePe struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	HTTP         *http.Client
}

func NewPhonePe(baseURL, clientID, clientSecret string) *PhonePe {
	return &PhonePe{
		BaseURL:      baseURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		HTTP:         &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *PhonePe) VerifyWebhook(ctx context.Context, webhookID string, h WebhookHeaders, rawBody []byte) (bool, error) {
	return false, errors.New("phonepe webhook verification not implemented")
}

func (p *PhonePe) GetTransaction(ctx context.Context, providerOrderID string) (*evidence.Transaction, error) {
	return nil, errors.New("phonepe get transaction not implemented")
}

func (p *PhonePe) Refund(ctx context.Context, captureID string, amountMinor int64, currency string, idempotencyKey string) (string, error) {
	return "", errors.New("phonepe refund not implemented")
}

func (p *PhonePe) ListWebhookEvents(ctx context.Context) ([]WebhookEvent, error) {
	return nil, nil
}

var _ = url.URL{}
var _ = time.Time{}
