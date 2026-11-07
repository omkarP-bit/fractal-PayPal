package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPayPalVerifyWebhookSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case "/v1/notifications/verify-webhook-signature":
			io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"verification_status":"SUCCESS"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := NewPayPal(srv.URL, "client-id", "client-secret", nil)
	p.HTTP = srv.Client()

	ok, err := p.VerifyWebhook(context.Background(), "wh_123", WebhookHeaders{
		TransmissionID:   "tid",
		TransmissionTime: "tt",
		TransmissionSig:  "sig",
		CertURL:          "https://cert.example.com",
		AuthAlgo:         "SHA256withRSA",
	}, []byte(`{"id":"evt_1"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected verification to succeed")
	}
}

func TestPayPalVerifyWebhookFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		case "/v1/notifications/verify-webhook-signature":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"verification_status":"FAILURE"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p := NewPayPal(srv.URL, "client-id", "client-secret", nil)
	p.HTTP = srv.Client()

	ok, err := p.VerifyWebhook(context.Background(), "wh_123", WebhookHeaders{
		TransmissionID:   "tid",
		TransmissionTime: "tt",
		TransmissionSig:  "sig",
		CertURL:          "https://cert.example.com",
		AuthAlgo:         "SHA256withRSA",
	}, []byte(`{"id":"evt_1"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected verification to fail")
	}
}

func TestPayPalVerifyWebhookServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/oauth2/token":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"access_token":"tok","expires_in":3600}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	p := NewPayPal(srv.URL, "client-id", "client-secret", nil)
	p.HTTP = srv.Client()

	_, err := p.VerifyWebhook(context.Background(), "wh_123", WebhookHeaders{
		TransmissionID:   "tid",
		TransmissionTime: "tt",
		TransmissionSig:  "sig",
		CertURL:          "https://cert.example.com",
		AuthAlgo:         "SHA256withRSA",
	}, []byte(`{"id":"evt_1"}`))
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestPayPalVerifyWebhookInvalidJSON(t *testing.T) {
	p := NewPayPal("https://api-m.sandbox.paypal.com", "client-id", "client-secret", nil)

	ok, err := p.VerifyWebhook(context.Background(), "wh_123", WebhookHeaders{
		TransmissionID:   "tid",
		TransmissionTime: "tt",
		TransmissionSig:  "sig",
		CertURL:          "https://cert.example.com",
		AuthAlgo:         "SHA256withRSA",
	}, []byte(`not json`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected invalid JSON to return false, nil")
	}
}

func TestPayPalTokenCaching(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"token-123","expires_in":3600}`))
	}))
	defer srv.Close()

	p := NewPayPal(srv.URL, "client-id", "client-secret", nil)
	p.HTTP = srv.Client()

	if _, err := p.accessToken(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := p.accessToken(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if calls != 1 {
		t.Errorf("expected 1 token request, got %d", calls)
	}
}

func TestPayPalAccessTokenExpired(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"token-","expires_in":1}`))
	}))
	defer srv.Close()

	p := NewPayPal(srv.URL, "client-id", "client-secret", nil)
	p.HTTP = srv.Client()

	if _, err := p.accessToken(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := p.accessToken(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if calls != 2 {
		t.Errorf("expected 2 token requests after expiry, got %d", calls)
	}
}
