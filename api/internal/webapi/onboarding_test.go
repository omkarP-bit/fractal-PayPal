package webapi

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"fractal/api/internal/provider"
)

func TestPublicBaseURLPrefersConfiguredHTTPSOrigin(t *testing.T) {
	h := &Handlers{PublicBaseURL: "https://app.example.com/"}
	req := httptest.NewRequest("POST", "http://localhost:8080/api/v1/onboarding/paypal", nil)

	base, err := h.publicBaseURL(req)
	if err != nil {
		t.Fatalf("publicBaseURL: %v", err)
	}
	if base != "https://app.example.com" {
		t.Fatalf("base = %q", base)
	}

	url, err := h.webhookURL(req, "acct-1")
	if err != nil {
		t.Fatalf("webhookURL: %v", err)
	}
	if url != "https://app.example.com/webhooks/paypal/acct-1" {
		t.Fatalf("webhookURL = %q", url)
	}
}

func TestPublicBaseURLRejectsLocalhostConfig(t *testing.T) {
	h := &Handlers{PublicBaseURL: "http://localhost:8080"}
	req := httptest.NewRequest("POST", "http://localhost:8080/api/v1/onboarding/paypal", nil)

	_, err := h.publicBaseURL(req)
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("expected an HTTPS error, got %v", err)
	}
}

func TestPublicBaseURLRejectsPlainLocalhostRequest(t *testing.T) {
	t.Setenv("PUBLIC_TUNNEL_URL", "")
	t.Setenv("RENDER_EXTERNAL_URL", "")
	h := &Handlers{}
	req := httptest.NewRequest("POST", "http://localhost:8080/api/v1/onboarding/paypal", nil)

	_, err := h.publicBaseURL(req)
	if err == nil {
		t.Fatal("expected localhost to be rejected")
	}
}

func TestPublicBaseURLUsesForwardedHTTPS(t *testing.T) {
	t.Setenv("PUBLIC_TUNNEL_URL", "")
	t.Setenv("RENDER_EXTERNAL_URL", "")
	h := &Handlers{}
	req := httptest.NewRequest("POST", "http://localhost:8080/api/v1/onboarding/paypal", nil)
	req.Host = "internal:8080"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "fractal.trycloudflare.com")

	base, err := h.publicBaseURL(req)
	if err != nil {
		t.Fatalf("publicBaseURL: %v", err)
	}
	if base != "https://fractal.trycloudflare.com" {
		t.Fatalf("base = %q", base)
	}
}

func TestPublicBaseURLFallsBackToTunnelEnv(t *testing.T) {
	t.Setenv("PUBLIC_TUNNEL_URL", "  https://tunnel.example.com/  ")
	h := &Handlers{}
	req := httptest.NewRequest("POST", "http://localhost:8080/api/v1/onboarding/paypal", nil)

	base, err := h.publicBaseURL(req)
	if err != nil {
		t.Fatalf("publicBaseURL: %v", err)
	}
	if base != "https://tunnel.example.com" {
		t.Fatalf("base = %q", base)
	}
}

func TestPublicBaseURLFallsBackToRenderExternalURL(t *testing.T) {
	t.Setenv("PUBLIC_TUNNEL_URL", "")
	t.Setenv("RENDER_EXTERNAL_URL", "  https://fractal-api.onrender.com/  ")
	h := &Handlers{}
	req := httptest.NewRequest("POST", "http://localhost:8080/api/v1/onboarding/paypal", nil)

	base, err := h.publicBaseURL(req)
	if err != nil {
		t.Fatalf("publicBaseURL: %v", err)
	}
	if base != "https://fractal-api.onrender.com" {
		t.Fatalf("base = %q", base)
	}
}

func TestPublicBaseURLRejectsPath(t *testing.T) {
	h := &Handlers{PublicBaseURL: "https://app.example.com/fractal"}
	req := httptest.NewRequest("POST", "http://localhost:8080/api/v1/onboarding/paypal", nil)

	if _, err := h.publicBaseURL(req); err == nil {
		t.Fatal("expected a path to be rejected")
	}
}

func TestIsPublicHTTPSURL(t *testing.T) {
	cases := map[string]bool{
		"https://app.example.com/webhooks/paypal/1": true,
		"http://app.example.com/webhooks/paypal/1":  false,
		"https://localhost/webhooks/paypal/1":       false,
		"https://127.0.0.1/webhooks/paypal/1":       false,
		"not a url":                                 false,
		"":                                          false,
	}
	for input, want := range cases {
		if got := isPublicHTTPSURL(input); got != want {
			t.Errorf("isPublicHTTPSURL(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestPlatformPayPalUsesEnvironment(t *testing.T) {
	t.Setenv("PAYPAL_CLIENT_ID", "platform-client-id")
	t.Setenv("PAYPAL_CLIENT_SECRET", "platform-secret")
	t.Setenv("PAYPAL_BASE_URL", "")

	h := &Handlers{}
	client, err := h.platformPayPal()
	if err != nil {
		t.Fatalf("platformPayPal: %v", err)
	}
	if client.ClientID != "platform-client-id" || client.ClientSecret != "platform-secret" {
		t.Fatal("platformPayPal did not read the deployment credentials")
	}
	if client.BaseURL != provider.PayPalSandboxBaseURL {
		t.Fatalf("base url = %q, want sandbox", client.BaseURL)
	}
}

func TestPlatformPayPalPrefersConfiguredClient(t *testing.T) {
	t.Setenv("PAYPAL_CLIENT_ID", "env-client")
	t.Setenv("PAYPAL_CLIENT_SECRET", "env-secret")

	configured := provider.NewPayPal(provider.PayPalSandboxBaseURL, "configured-client", "configured-secret", nil)
	h := &Handlers{PayPal: configured}

	client, err := h.platformPayPal()
	if err != nil {
		t.Fatalf("platformPayPal: %v", err)
	}
	if client != configured {
		t.Fatal("platformPayPal should reuse the client built at startup")
	}
}

func TestPlatformPayPalRequiresCredentials(t *testing.T) {
	t.Setenv("PAYPAL_CLIENT_ID", "")
	t.Setenv("PAYPAL_CLIENT_SECRET", "")

	h := &Handlers{}
	if _, err := h.platformPayPal(); err == nil {
		t.Fatal("expected missing deployment credentials to fail")
	}
}

func TestIsWebhookLimitError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"limit name", &provider.APIError{Status: 400, Name: "WEBHOOK_LIMIT_EXCEEDED"}, true},
		{"limit message", &provider.APIError{Status: 400, Name: "VALIDATION_ERROR", Message: "The maximum number of webhooks has been reached"}, true},
		{"other error", &provider.APIError{Status: 401, Name: "invalid_client"}, false},
		{"plain error", errors.New("boom"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isWebhookLimitError(tc.err); got != tc.want {
				t.Fatalf("isWebhookLimitError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestManagedExternalAccountIDIsMerchantScoped(t *testing.T) {
	first := managedExternalAccountID("11111111-1111-1111-1111-111111111111")
	second := managedExternalAccountID("22222222-2222-2222-2222-222222222222")
	if first == second {
		t.Fatal("managed external account IDs must be unique per merchant")
	}
	if !strings.HasPrefix(first, "PLATFORM:") {
		t.Fatalf("managed external account ID = %q", first)
	}
}
