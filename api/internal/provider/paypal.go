package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	tokenEndpoint = "/v1/oauth2/token"
	ordersEndpoint = "/v2/checkout/orders"
	capturesEndpoint = "/v2/payments/captures"
	verifyWebhookEndpoint = "/v1/notifications/verify-webhook-signature"
)

// PayPalProvider calls PayPal REST directly. No SDK.
type PayPalProvider struct {
	clientID     string
	clientSecret string
	baseURL      string
	httpClient   *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

func NewPayPal(clientID, clientSecret, baseURL string) *PayPalProvider {
	return &PayPalProvider{
		clientID:     clientID,
		clientSecret: clientSecret,
		baseURL:      strings.TrimRight(baseURL, "/"),
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *PayPalProvider) GetTransaction(ctx context.Context, providerOrderID string) (*Transaction, error) {
	// Capture lookup via /v2/payments/captures/{capture_id} is used when we have a capture id.
	// For order lookup we use /v2/checkout/orders/{id}. Here we fetch by order id.
	return p.getOrder(ctx, providerOrderID)
}

func (p *PayPalProvider) getOrder(ctx context.Context, orderID string) (*Transaction, error) {
	token, err := p.accessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+ordersEndpoint+"/"+orderID, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("paypal order %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		ID            string `json:"id"`
		Status        string `json:"status"`
		PurchaseUnits []struct {
			Amount struct {
				Value    string `json:"value"`
				Currency string `json:"currency_code"`
			} `json:"amount"`
		} `json:"purchase_units"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	amountMinor := int64(0)
	if len(parsed.PurchaseUnits) > 0 {
		amountMinor = parseMinor(parsed.PurchaseUnits[0].Amount.Value)
	}
	return &Transaction{
		Provider:         "PAYPAL",
		ProviderOrderID:  parsed.ID,
		AmountMinor:      amountMinor,
		Currency:         parsed.PurchaseUnits[0].Amount.Currency,
		Status:           parsed.Status,
		FetchedAt:        time.Now().UTC(),
	}, nil
}

func (p *PayPalProvider) VerifyWebhook(ctx context.Context, rawBody []byte, headers map[string]string) (bool, error) {
	token, err := p.accessToken(ctx)
	if err != nil {
		return false, fmt.Errorf("get token: %w", err)
	}
	body := map[string]any{
		"transmission_id": headers["Paypal-Transmission-Id"],
		"transmission_time": headers["Paypal-Transmission-Time"],
		"cert_url":         headers["Paypal-Cert-Url"],
		"auth_algo":        headers["Paypal-Auth-Algo"],
		"transmission_sig": headers["Paypal-Transmission-Sig"],
		"webhook_id":       headers["Paypal-Webhook-Id"],
		"webhook_event":    json.RawMessage(rawBody),
	}
	reqBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+verifyWebhookEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("verify request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("verify %d: %s", resp.StatusCode, string(respBody))
	}
	var result struct {
		VerificationStatus string `json:"verification_status"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return false, fmt.Errorf("parse verify: %w", err)
	}
	return result.VerificationStatus == "SUCCESS", nil
}

func (p *PayPalProvider) Refund(ctx context.Context, captureID string, amountMinor int64, currency string, idempotencyKey string) (string, error) {
	return "", fmt.Errorf("refund not implemented in this step")
}

func (p *PayPalProvider) ListWebhookEvents(ctx context.Context) ([]WebhookEvent, error) {
	return nil, fmt.Errorf("list webhook events not implemented in this step")
}

func (p *PayPalProvider) accessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && time.Now().Add(30*time.Second).Before(p.tokenExp) {
		return p.token, nil
	}
	data := url.Values{"grant_type": {"client_credentials"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(p.clientID, p.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("parse token: %w", err)
	}
	p.token = parsed.AccessToken
	p.tokenExp = time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second)
	return p.token, nil
}

func parseMinor(value string) int64 {
	var neg int64 = 1
	v := value
	if strings.HasPrefix(v, "-") {
		neg = -1
		v = strings.TrimPrefix(v, "-")
	}
	parts := strings.SplitN(v, ".", 2)
	major := parts[0]
	minor := "00"
	if len(parts) == 2 {
		minor = parts[1]
		if len(minor) == 1 {
			minor += "0"
		}
		if len(minor) > 2 {
			minor = minor[:2]
		}
	}
	n := int64(0)
	for _, ch := range major {
		if ch >= '0' && ch <= '9' {
			n = n*10 + int64(ch-'0')
		}
	}
	n = n*100
	for _, ch := range minor {
		if ch >= '0' && ch <= '9' {
			n = n*10 + int64(ch-'0')
		}
	}
	return n * neg
}

