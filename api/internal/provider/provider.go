// Package provider defines the payment-provider boundary. PayPal is the first
// implementation. Other providers can be added without touching callers.
package provider

import "context"

// WebhookHeaders are the signature-related headers PayPal sends with every
// webhook delivery.
type WebhookHeaders struct {
	TransmissionID   string
	TransmissionTime string
	TransmissionSig  string
	CertURL          string
	AuthAlgo         string
}

// Complete reports whether every header needed for verification is present.
func (h WebhookHeaders) Complete() bool {
	return h.TransmissionID != "" && h.TransmissionTime != "" &&
		h.TransmissionSig != "" && h.CertURL != "" && h.AuthAlgo != ""
}

// WebhookVerifier confirms that a webhook really came from the provider.
//
// Return contract (callers depend on it):
//
//	(true,  nil)  the provider confirmed authenticity
//	(false, nil)  definitive failure: the event is not authentic
//	(false, err)  undecided (network error, provider 5xx, token failure).
//	              The caller must NOT store the event and must return a
//	              retryable status so the provider redelivers.
type WebhookVerifier interface {
	VerifyWebhook(ctx context.Context, webhookID string, h WebhookHeaders, rawBody []byte) (bool, error)
}
