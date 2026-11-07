package webhook

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"fractal/api/internal/provider"
)

const testAccountID = "550e8400-e29b-41d4-a716-446655440000"
const testWebhookID = "wh-550e8400-e29b-41d4-a716-446655440000"

// ----- mock AccountResolver -----

type mockAccountResolver struct {
	webhookID string
	err       error
}

func (m *mockAccountResolver) WebhookID(ctx context.Context, providerAccountID string) (string, error) {
	return m.webhookID, m.err
}

// ----- mock Store -----

type mockStore struct {
	inserted  bool
	insertErr error
	secErr    error
	calls     int
}

func (m *mockStore) InsertVerified(ctx context.Context, accountID string, ev Event, raw []byte, sha string) (bool, error) {
	m.calls++
	if m.insertErr != nil {
		return false, m.insertErr
	}
	return m.inserted, nil
}

func (m *mockStore) RecordSecurityEvent(ctx context.Context, se SecurityEvent) error {
	m.calls++
	return m.secErr
}

// ----- mock Verifier -----

type mockVerifier struct {
	ok  bool
	err error
}

func (m *mockVerifier) VerifyWebhook(ctx context.Context, webhookID string, h provider.WebhookHeaders, rawBody []byte) (bool, error) {
	return m.ok, m.err
}

// routeThroughMux wraps a handler in a test ServeMux so that r.PathValue()
// is populated (Go 1.22+ ServeMux sets path values in the request context).
func routeThroughMux(h http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /api/v1/webhooks/paypal/{provider_account_id}", h)
	return mux
}

// headers returns a map of all PayPal signature headers.
func headers() map[string]string {
	return map[string]string{
		"PAYPAL-TRANSMISSION-ID":   "tid",
		"PAYPAL-TRANSMISSION-TIME": "tt",
		"PAYPAL-TRANSMISSION-SIG":  "sig",
		"PAYPAL-CERT-URL":          "https://cert",
		"PAYPAL-AUTH-ALGO":         "SHA256withRSA",
	}
}

// reqWithHeaders creates a POST request with the given body and headers.
func reqWithHeaders(body string, hdrs map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/paypal/"+testAccountID, strings.NewReader(body))
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	return req
}

// ----- WebhookHeaders tests -----

func TestWebhookHeadersComplete(t *testing.T) {
	t.Run("all headers present", func(t *testing.T) {
		h := provider.WebhookHeaders{
			TransmissionID:   "tid",
			TransmissionTime: "tt",
			TransmissionSig:  "sig",
			CertURL:          "https://cert",
			AuthAlgo:         "SHA256withRSA",
		}
		if !h.Complete() {
			t.Error("expected Complete() to be true")
		}
	})

	t.Run("missing transmission id", func(t *testing.T) {
		h := provider.WebhookHeaders{TransmissionTime: "tt", TransmissionSig: "sig", CertURL: "https://cert", AuthAlgo: "SHA256withRSA"}
		if h.Complete() {
			t.Error("expected Complete() to be false")
		}
	})

	t.Run("missing all headers", func(t *testing.T) {
		h := provider.WebhookHeaders{}
		if h.Complete() {
			t.Error("expected Complete() to be false")
		}
	})
}

// ----- Handler tests -----

func TestHandlerMissingHeaders(t *testing.T) {
	store := &mockStore{}
	h := routeThroughMux(&Handler{
		Provider: "PAYPAL",
		Accounts: &mockAccountResolver{webhookID: testWebhookID},
		Verifier: &mockVerifier{},
		Store:    store,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/paypal/"+testAccountID, strings.NewReader(`{"id":"evt_1"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	// Missing headers go through reject() which records a security event
	if store.calls != 1 {
		t.Errorf("expected 1 security event call, got %d", store.calls)
	}
}

func TestHandlerUnknownAccount(t *testing.T) {
	store := &mockStore{}
	h := routeThroughMux(&Handler{
		Provider: "PAYPAL",
		Accounts: &mockAccountResolver{err: ErrAccountNotFound},
		Verifier: &mockVerifier{},
		Store:    store,
	})

	req := reqWithHeaders(`{"id":"evt_1"}`, headers())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestHandlerInvalidSignature(t *testing.T) {
	store := &mockStore{}
	h := routeThroughMux(&Handler{
		Provider: "PAYPAL",
		Accounts: &mockAccountResolver{webhookID: testWebhookID},
		Verifier: &mockVerifier{ok: false},
		Store:    store,
	})

	req := reqWithHeaders(`{"id":"evt_1"}`, headers())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	if store.calls != 1 {
		t.Errorf("expected 1 security event call, got %d", store.calls)
	}
}

func TestHandlerVerificationUndecided(t *testing.T) {
	store := &mockStore{}
	h := routeThroughMux(&Handler{
		Provider: "PAYPAL",
		Accounts: &mockAccountResolver{webhookID: testWebhookID},
		Verifier: &mockVerifier{err: errors.New("network down")},
		Store:    store,
	})

	req := reqWithHeaders(`{"id":"evt_1"}`, headers())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestHandlerAcceptedNewEvent(t *testing.T) {
	store := &mockStore{inserted: true}
	handler := &Handler{
		Provider: "PAYPAL",
		Accounts: &mockAccountResolver{webhookID: testWebhookID},
		Verifier: &mockVerifier{ok: true},
		Store:    store,
	}
	wakeCalled := false
	handler.Wake = func() { wakeCalled = true }
	h := routeThroughMux(handler)

	body := `{"id":"evt_1","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"cap_1"}}`
	req := reqWithHeaders(body, headers())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !wakeCalled {
		t.Error("expected Wake() to be called for new event")
	}
}

func TestHandlerDuplicateEvent(t *testing.T) {
	store := &mockStore{inserted: false}
	handler := &Handler{
		Provider: "PAYPAL",
		Accounts: &mockAccountResolver{webhookID: testWebhookID},
		Verifier: &mockVerifier{ok: true},
		Store:    store,
	}
	wakeCalled := false
	handler.Wake = func() { wakeCalled = true }
	h := routeThroughMux(handler)

	body := `{"id":"evt_1","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"cap_1"}}`
	req := reqWithHeaders(body, headers())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if wakeCalled {
		t.Error("expected Wake() NOT to be called for duplicate")
	}
}

func TestHandlerMalformedEvent(t *testing.T) {
	store := &mockStore{}
	h := routeThroughMux(&Handler{
		Provider: "PAYPAL",
		Accounts: &mockAccountResolver{webhookID: testWebhookID},
		Verifier: &mockVerifier{ok: true},
		Store:    store,
	})

	body := `{"foo":"bar"}`
	req := reqWithHeaders(body, headers())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandlerBodyTooLarge(t *testing.T) {
	store := &mockStore{}
	h := routeThroughMux(&Handler{
		Provider: "PAYPAL",
		Accounts: &mockAccountResolver{webhookID: testWebhookID},
		Verifier: &mockVerifier{ok: true},
		Store:    store,
	})

	bigBody := strings.Repeat("a", 2*1024*1024)
	req := reqWithHeaders(bigBody, headers())
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", rr.Code)
	}
}
