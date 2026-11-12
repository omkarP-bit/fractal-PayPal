package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"fractal/api/internal/evidence"
)

const (
	PayPalSandboxBaseURL = "https://api-m.sandbox.paypal.com"
	PayPalLiveBaseURL    = "https://api-m.paypal.com"
)

// PayPal talks to PayPal's REST API directly over HTTPS (no SDK).
type PayPal struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	HTTP         *http.Client
	Log          *slog.Logger

	mu      sync.Mutex
	token   string
	expires time.Time
}

func NewPayPal(baseURL, clientID, clientSecret string, log *slog.Logger) *PayPal {
	if log == nil {
		log = slog.Default()
	}
	return &PayPal{
		BaseURL:      strings.TrimRight(baseURL, "/"),
		ClientID:     clientID,
		ClientSecret: clientSecret,
		HTTP:         &http.Client{Timeout: 10 * time.Second},
		Log:          log,
	}
}

// accessToken returns a cached OAuth token, refreshing it shortly before expiry.
// The mutex is held during the fetch so concurrent callers share one refresh.
func (p *PayPal) accessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && time.Now().Add(30*time.Second).Before(p.expires) {
		return p.token, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/v1/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(p.ClientID, p.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("paypal token request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("paypal token request: status %d", resp.StatusCode)
	}
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil || out.AccessToken == "" {
		return "", errors.New("paypal token response invalid")
	}
	p.token = out.AccessToken
	p.expires = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	return p.token, nil
}

func (p *PayPal) clearToken() {
	p.mu.Lock()
	p.token = ""
	p.mu.Unlock()
}

// VerifyWebhook posts the event back to PayPal's verify-webhook-signature
// endpoint (the "postback" method).
//
// Notes:
//   - The request body is assembled by hand so the original event bytes are
//     embedded unchanged under "webhook_event" (no re-serialization of the event).
//   - cert_url comes from an attacker-controllable header. We never fetch it
//     ourselves; only PayPal does, which avoids SSRF from our side.
//   - Simulator (mock) events cannot be verified and will come back as failures.
func (p *PayPal) VerifyWebhook(ctx context.Context, webhookID string, h WebhookHeaders, rawBody []byte) (bool, error) {
	if !json.Valid(rawBody) {
		return false, nil
	}
	tok, err := p.accessToken(ctx)
	if err != nil {
		return false, err
	}

	str := func(s string) []byte { b, _ := json.Marshal(s); return b }
	var body bytes.Buffer
	body.WriteString(`{"auth_algo":`)
	body.Write(str(h.AuthAlgo))
	body.WriteString(`,"cert_url":`)
	body.Write(str(h.CertURL))
	body.WriteString(`,"transmission_id":`)
	body.Write(str(h.TransmissionID))
	body.WriteString(`,"transmission_sig":`)
	body.Write(str(h.TransmissionSig))
	body.WriteString(`,"transmission_time":`)
	body.Write(str(h.TransmissionTime))
	body.WriteString(`,"webhook_id":`)
	body.Write(str(webhookID))
	body.WriteString(`,"webhook_event":`)
	body.Write(rawBody)
	body.WriteString(`}`)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/v1/notifications/verify-webhook-signature", &body)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.HTTP.Do(req)
	if err != nil {
		return false, fmt.Errorf("paypal verify request: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		p.clearToken() // token may have been revoked; next attempt fetches a new one
		return false, errors.New("paypal verify: unauthorized (token cleared)")
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return false, fmt.Errorf("paypal verify: status %d", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		// Other 4xx (for example a malformed or forged request): not authentic.
		p.Log.Warn("paypal verify rejected request", "status", resp.StatusCode)
		return false, nil
	}

	var out struct {
		VerificationStatus string `json:"verification_status"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return false, fmt.Errorf("paypal verify response: %w", err)
	}
	return out.VerificationStatus == "SUCCESS", nil
}

func (p *PayPal) GetTransaction(ctx context.Context, providerOrderID string) (*evidence.Transaction, error) {
	tok, err := p.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("paypal access token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.BaseURL+"/v2/checkout/orders/"+url.PathEscape(providerOrderID), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("paypal get order: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("paypal order not found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("paypal get order: status %d", resp.StatusCode)
	}

	var out struct {
		ID            string `json:"id"`
		Status        string `json:"status"`
		PurchaseUnits []struct {
			InvoiceID string `json:"invoice_id"`
			Amount    struct {
				Value        string `json:"value"`
				CurrencyCode string `json:"currency_code"`
			} `json:"amount"`
			Payments struct {
				Captures []struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"captures"`
			} `json:"payments"`
		} `json:"purchase_units"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("paypal get order response: %w", err)
	}

	amountMinor := int64(0)
	currency := "USD"
	merchantOrderRef := ""
	if len(out.PurchaseUnits) > 0 {
		merchantOrderRef = out.PurchaseUnits[0].InvoiceID
		currency = out.PurchaseUnits[0].Amount.CurrencyCode
		if currency == "" {
			currency = "USD"
		}
		amountMinor, err = amountStringToMinor(out.PurchaseUnits[0].Amount.Value)
		if err != nil {
			return nil, fmt.Errorf("paypal amount parse: %w", err)
		}
	}
	captureID := ""
	if len(out.PurchaseUnits) > 0 && len(out.PurchaseUnits[0].Payments.Captures) > 0 {
		captureID = out.PurchaseUnits[0].Payments.Captures[0].ID
	}

	return &evidence.Transaction{
		Provider:         "PAYPAL",
		ProviderOrderID:  out.ID,
		CaptureID:        captureID,
		MerchantOrderRef: merchantOrderRef,
		AmountMinor:      amountMinor,
		Currency:         currency,
		Status:           out.Status,
		FetchedAt:        time.Now().UTC(),
	}, nil
}

func amountStringToMinor(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty amount")
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	parts := strings.SplitN(s, ".", 2)
	whole := parts[0]
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
	}
	if !isNumeric(whole) {
		return 0, fmt.Errorf("invalid whole part: %q", whole)
	}
	for _, c := range frac {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid fraction part: %q", frac)
		}
	}
	switch {
	case len(frac) == 0:
		whole += "00"
	case len(frac) == 1:
		whole += frac + "0"
	case len(frac) == 2:
		whole += frac
	default:
		whole += frac[:2]
	}
	v, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, err
	}
	if neg {
		v = -v
	}
	return v, nil
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (p *PayPal) Refund(ctx context.Context, captureID string, amountMinor int64, currency string, idempotencyKey string) (string, error) {
	tok, err := p.accessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("paypal access token: %w", err)
	}
	body := map[string]any{
		"amount": map[string]any{
			"value":         amountStringFromMinor(amountMinor),
			"currency_code": currency,
		},
	}
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.BaseURL+"/v2/payments/captures/"+url.PathEscape(captureID)+"/refund", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("PayPal-Request-Id", idempotencyKey)
	}
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("paypal refund: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("paypal refund: status %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", fmt.Errorf("paypal refund response: %w", err)
	}
	return out.ID, nil
}

func amountStringFromMinor(amountMinor int64) string {
	whole := amountMinor / 100
	frac := amountMinor % 100
	if frac < 0 {
		frac = -frac
	}
	return fmt.Sprintf("%d.%02d", whole, frac)
}

// WebhookEventTypes is the subscription set used for merchant webhooks.
// "*" lets PayPal deliver every event type; FRACTAL only stores/processes the
// ones it understands, and the signature is verified before anything is kept.
var WebhookEventTypes = []string{"*"}

// Webhook is the subset of PayPal's webhook resource FRACTAL stores.
type Webhook struct {
	ID         string             `json:"id"`
	URL        string             `json:"url"`
	EventTypes []WebhookEventType `json:"event_types"`
}

type WebhookEventType struct {
	Name string `json:"name"`
}

// APIError describes a non-2xx PayPal response without exposing credentials.
type APIError struct {
	Status  int
	Name    string
	Message string
}

func (e *APIError) Error() string {
	if e.Name != "" || e.Message != "" {
		return fmt.Sprintf("paypal api: status %d: %s: %s", e.Status, e.Name, e.Message)
	}
	return fmt.Sprintf("paypal api: status %d", e.Status)
}

func (p *PayPal) webhookRequest(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	tok, err := p.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	return p.HTTP.Do(req)
}

func decodePayPalError(resp *http.Response) error {
	defer resp.Body.Close()
	var out struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&out)
	return &APIError{Status: resp.StatusCode, Name: out.Name, Message: out.Message}
}

// ListWebhooks returns the webhooks registered for the credentials' PayPal app.
func (p *PayPal) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	resp, err := p.webhookRequest(ctx, http.MethodGet, "/v1/notifications/webhooks", nil)
	if err != nil {
		return nil, fmt.Errorf("paypal list webhooks: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, decodePayPalError(resp)
	}
	defer resp.Body.Close()
	var out struct {
		Webhooks []Webhook `json:"webhooks"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("paypal list webhooks response: %w", err)
	}
	return out.Webhooks, nil
}

// CreateWebhook registers a webhook URL and returns PayPal's webhook ID.
func (p *PayPal) CreateWebhook(ctx context.Context, webhookURL string, eventTypes []string) (*Webhook, error) {
	if strings.TrimSpace(webhookURL) == "" {
		return nil, errors.New("webhook url is empty")
	}
	if len(eventTypes) == 0 {
		eventTypes = WebhookEventTypes
	}
	types := make([]WebhookEventType, 0, len(eventTypes))
	for _, name := range eventTypes {
		types = append(types, WebhookEventType{Name: name})
	}
	body := map[string]any{"url": webhookURL, "event_types": types}

	resp, err := p.webhookRequest(ctx, http.MethodPost, "/v1/notifications/webhooks", body)
	if err != nil {
		return nil, fmt.Errorf("paypal create webhook: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, decodePayPalError(resp)
	}
	defer resp.Body.Close()
	var out Webhook
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, fmt.Errorf("paypal create webhook response: %w", err)
	}
	if out.ID == "" {
		return nil, errors.New("paypal create webhook response missing id")
	}
	return &out, nil
}

// DeleteWebhook removes a webhook. A missing webhook is not an error.
func (p *PayPal) DeleteWebhook(ctx context.Context, webhookID string) error {
	if strings.TrimSpace(webhookID) == "" {
		return nil
	}
	resp, err := p.webhookRequest(ctx, http.MethodDelete,
		"/v1/notifications/webhooks/"+url.PathEscape(webhookID), nil)
	if err != nil {
		return fmt.Errorf("paypal delete webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return decodePayPalError(resp)
}

// EnsureWebhook reuses a webhook already registered for the same URL, otherwise
// creates one. Re-running Connect PayPal therefore stays idempotent.
func (p *PayPal) EnsureWebhook(ctx context.Context, webhookURL string) (*Webhook, error) {
	existing, err := p.ListWebhooks(ctx)
	if err == nil {
		for i := range existing {
			if sameWebhookURL(existing[i].URL, webhookURL) {
				return &existing[i], nil
			}
		}
	}
	return p.CreateWebhook(ctx, webhookURL, WebhookEventTypes)
}

func sameWebhookURL(a, b string) bool {
	return strings.TrimRight(strings.TrimSpace(a), "/") == strings.TrimRight(strings.TrimSpace(b), "/")
}

// BaseURLForEnvironment returns the PayPal REST endpoint for a credential set.
func BaseURLForEnvironment(environment string) string {
	if strings.EqualFold(environment, "LIVE") {
		return PayPalLiveBaseURL
	}
	return PayPalSandboxBaseURL
}
