// Package secrets encrypts provider credentials before they are stored.
//
// The application key lives in environment configuration (never in the
// database, never in an API response and never in a log line). Values are
// sealed with AES-256-GCM and versioned so the format can change later.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	envKey        = "FRACTAL_ENCRYPTION_KEY"
	versionPrefix = "v1:"
)

var (
	ErrNoKey        = errors.New("FRACTAL_ENCRYPTION_KEY is not configured")
	ErrNotSealed    = errors.New("stored secret is not in the sealed format")
	ErrCorruptValue = errors.New("stored secret could not be decrypted")
)

// Cipher seals and opens short credential strings.
type Cipher struct {
	aead cipher.AEAD
}

// New builds a Cipher from a base64 32-byte key, a 32-byte raw key, or any
// passphrase (hashed to 32 bytes). The passphrase path keeps local development
// friction low; production deployments should set a random 32-byte key.
func New(key string) (*Cipher, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrNoKey
	}

	var raw []byte
	if decoded, err := base64.StdEncoding.DecodeString(key); err == nil && len(decoded) == 32 {
		raw = decoded
	} else if len(key) == 32 {
		raw = []byte(key)
	} else {
		sum := sha256.Sum256([]byte(key))
		raw = sum[:]
	}

	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// NewFromEnv reads FRACTAL_ENCRYPTION_KEY. As a development-only convenience it
// falls back to FRACTAL_JWT_SECRET so an existing local .env keeps working.
func NewFromEnv() (*Cipher, error) {
	key := os.Getenv(envKey)
	if key == "" {
		key = os.Getenv("FRACTAL_JWT_SECRET")
	}
	return New(key)
}

// Encrypt seals plaintext and returns a versioned, base64-encoded value.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", errors.New("cannot encrypt an empty secret")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, sealed...)
	return versionPrefix + base64.RawStdEncoding.EncodeToString(payload), nil
}

// Decrypt opens a value produced by Encrypt.
func (c *Cipher) Decrypt(value string) (string, error) {
	if !strings.HasPrefix(value, versionPrefix) {
		return "", ErrNotSealed
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, versionPrefix))
	if err != nil {
		return "", ErrCorruptValue
	}
	nonceSize := c.aead.NonceSize()
	if len(payload) <= nonceSize {
		return "", ErrCorruptValue
	}
	plaintext, err := c.aead.Open(nil, payload[:nonceSize], payload[nonceSize:], nil)
	if err != nil {
		return "", ErrCorruptValue
	}
	return string(plaintext), nil
}
