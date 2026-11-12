package webapi

import (
	"net/http/httptest"
	"strings"
	"testing"
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
	h := &Handlers{}
	req := httptest.NewRequest("POST", "http://localhost:8080/api/v1/onboarding/paypal", nil)

	_, err := h.publicBaseURL(req)
	if err == nil {
		t.Fatal("expected localhost to be rejected")
	}
}

func TestPublicBaseURLUsesForwardedHTTPS(t *testing.T) {
	t.Setenv("PUBLIC_TUNNEL_URL", "")
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
