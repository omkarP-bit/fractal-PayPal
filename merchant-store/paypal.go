package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type MPayPal struct {
	clientID     string
	clientSecret string
	baseURL      string
	httpClient   *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

func NewMPayPal(clientID, clientSecret, baseURL string) *MPayPal {
	return &MPayPal{
		clientID:     clientID,
		clientSecret: clientSecret,
		baseURL:      strings.TrimRight(baseURL, "/"),
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *MPayPal) CreateOrder(amountMinor int64, currency, invoiceID, returnURL, cancelURL string) (string, string, error) {
	token, err := p.accessToken()
	if err != nil {
		return "", "", fmt.Errorf("token: %w", err)
	}
	body := map[string]any{
		"intent": "CAPTURE",
		"purchase_units": []map[string]any{
			{
				"invoice_id": invoiceID,
				"amount": map[string]any{
					"value":         fmt.Sprintf("%.2f", float64(amountMinor)/100),
					"currency_code": currency,
				},
			},
		},
		"application_context": map[string]any{
			"return_url": returnURL,
			"cancel_url": cancelURL,
		},
	}
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, p.baseURL+"/v2/checkout/orders", strings.NewReader(string(b)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", "", fmt.Errorf("create order %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		ID    string `json:"id"`
		Links []struct {
			Rel    string `json:"rel"`
			Href   string `json:"href"`
			Method string `json:"method"`
		} `json:"links"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", "", err
	}
	approveURL := ""
	for _, l := range parsed.Links {
		if l.Rel == "approve" {
			approveURL = l.Href
		}
	}
	return parsed.ID, approveURL, nil
}

type captureResult struct {
	ID     string
	Status string
}

func parseCaptures(raw []byte) (captureResult, error) {
	var parsed struct {
		ID            string `json:"id"`
		Status        string `json:"status"`
		PurchaseUnits []struct {
			Payments struct {
				Captures []struct {
					ID     string `json:"id"`
					Status string `json:"status"`
				} `json:"captures"`
			} `json:"payments"`
		} `json:"purchase_units"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return captureResult{}, err
	}
	if len(parsed.PurchaseUnits) > 0 && len(parsed.PurchaseUnits[0].Payments.Captures) > 0 {
		capture := parsed.PurchaseUnits[0].Payments.Captures[0]
		return captureResult{ID: capture.ID, Status: capture.Status}, nil
	}
	if parsed.Status == "COMPLETED" {
		return captureResult{}, fmt.Errorf("order %s is COMPLETED but has no captures", parsed.ID)
	}
	return captureResult{}, fmt.Errorf("order %s is %s, not captured", parsed.ID, parsed.Status)
}

func (p *MPayPal) CaptureOrder(orderID string) (captureID string, status string, err error) {
	token, err := p.accessToken()
	if err != nil {
		return "", "", fmt.Errorf("token: %w", err)
	}
	req, _ := http.NewRequest(http.MethodPost, p.baseURL+"/v2/checkout/orders/"+orderID+"/capture", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		result, err := parseCaptures(raw)
		if err != nil {
			return "", "", err
		}
		return result.ID, result.Status, nil
	}
	if resp.StatusCode == http.StatusUnprocessableEntity && strings.Contains(string(raw), "ORDER_ALREADY_CAPTURED") {
		return p.GetCapturedOrder(orderID)
	}
	return "", "", fmt.Errorf("capture %d: %s", resp.StatusCode, string(raw))
}

func (p *MPayPal) GetCapturedOrder(orderID string) (captureID string, status string, err error) {
	token, err := p.accessToken()
	if err != nil {
		return "", "", fmt.Errorf("token: %w", err)
	}
	req, _ := http.NewRequest(http.MethodGet, p.baseURL+"/v2/checkout/orders/"+orderID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("get order %d: %s", resp.StatusCode, string(raw))
	}
	result, err := parseCaptures(raw)
	if err != nil {
		return "", "", err
	}
	return result.ID, result.Status, nil
}

func (p *MPayPal) accessToken() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && time.Now().Add(30*time.Second).Before(p.tokenExp) {
		return p.token, nil
	}
	data := url.Values{"grant_type": {"client_credentials"}}
	req, _ := http.NewRequest(http.MethodPost, p.baseURL+"/v1/oauth2/token", strings.NewReader(data.Encode()))
	req.SetBasicAuth(p.clientID, p.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token %d: %s", resp.StatusCode, string(raw))
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	p.token = parsed.AccessToken
	p.tokenExp = time.Now().Add(time.Duration(parsed.ExpiresIn) * time.Second)
	return p.token, nil
}
