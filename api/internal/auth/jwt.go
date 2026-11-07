package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// Dev-only JWT issuer. Production must use a real OIDC provider and key rotation.

type Claims struct {
	Sub         string   `json:"sub"`
	Kind        string   `json:"kind"`
	MerchantID  string   `json:"merchant_id,omitempty"`
	Role        string   `json:"role,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Exp         int64    `json:"exp"`
}

func Secret() []byte {
	return []byte(os.Getenv("FRACTAL_JWT_SECRET"))
}

func Sign(claims Claims) (string, error) {
	claims.Exp = time.Now().Add(24 * time.Hour).Unix()
	header := b64([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := b64(mustJSON(claims))
	sig := b64(hmacSha256([]byte(header+"."+payload), Secret()))
	return header + "." + payload + "." + sig, nil
}

func Verify(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed token")
	}
	expected := b64(hmacSha256([]byte(parts[0]+"."+parts[1]), Secret()))
	if !hmac.Equal([]byte(expected), []byte(parts[2])) {
		return nil, fmt.Errorf("bad signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, err
	}
	if c.Exp != 0 && time.Now().Unix() > c.Exp {
		return nil, fmt.Errorf("expired")
	}
	return &c, nil
}

func b64(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func hmacSha256(data, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
