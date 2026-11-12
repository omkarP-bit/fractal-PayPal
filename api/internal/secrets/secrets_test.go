package secrets

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c, err := New("test-key")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	sealed, err := c.Encrypt("client-secret-value")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(sealed, versionPrefix) {
		t.Fatalf("sealed value missing version prefix: %q", sealed)
	}
	if strings.Contains(sealed, "client-secret-value") {
		t.Fatal("sealed value contains the plaintext")
	}

	opened, err := c.Decrypt(sealed)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if opened != "client-secret-value" {
		t.Fatalf("Decrypt = %q, want %q", opened, "client-secret-value")
	}
}

func TestEncryptUsesFreshNonce(t *testing.T) {
	c, _ := New("test-key")
	first, _ := c.Encrypt("same")
	second, _ := c.Encrypt("same")
	if first == second {
		t.Fatal("two encryptions of the same value must not be identical")
	}
}

func TestDecryptRejectsTampering(t *testing.T) {
	c, _ := New("test-key")
	sealed, _ := c.Encrypt("value")
	tampered := sealed[:len(sealed)-2] + "AA"
	if _, err := c.Decrypt(tampered); err == nil {
		t.Fatal("expected tampered value to fail")
	}
}

func TestDecryptRejectsPlaintext(t *testing.T) {
	c, _ := New("test-key")
	if _, err := c.Decrypt("not-sealed"); err != ErrNotSealed {
		t.Fatalf("expected ErrNotSealed, got %v", err)
	}
}

func TestNewRequiresKey(t *testing.T) {
	if _, err := New(""); err != ErrNoKey {
		t.Fatalf("expected ErrNoKey, got %v", err)
	}
}
