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
	"strings"
	"sync"
	"time"
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
