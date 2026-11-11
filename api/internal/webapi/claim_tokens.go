package webapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const claimTokenLifetime = 24 * time.Hour

type claimTokenRequest struct {
	MerchantID       string `json:"merchant_id"`
	MerchantOrderRef string `json:"merchant_order_ref"`
	CustomerEmail    string `json:"customer_email"`
	Token            string `json:"token"`
	AmountMinor      int64  `json:"amount_minor"`
	Currency         string `json:"currency"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}

func (h *Handlers) RegisterClaimToken(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request", "The claim token body could not be read.")
		return
	}
	if !h.verifyMerchantSignature(r, raw) {
		writeProblem(w, http.StatusUnauthorized, "Unauthenticated", "The merchant signature is missing or invalid.")
		return
	}

	var req claimTokenRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request", "Expected a JSON claim token body.")
		return
	}

	merchantID := strings.TrimSpace(req.MerchantID)
	if _, ok := uuidText(merchantID); !ok {
		writeProblem(w, http.StatusBadRequest, "Invalid merchant", "The merchant id is not valid.")
		return
	}
	orderRef := strings.TrimSpace(req.MerchantOrderRef)
	email := strings.ToLower(strings.TrimSpace(req.CustomerEmail))
	token := strings.TrimSpace(req.Token)
	if orderRef == "" || email == "" || token == "" {
		writeProblem(w, http.StatusBadRequest, "Missing fields", "Order reference, customer email and token are required.")
		return
	}

	lifetime := claimTokenLifetime
	if req.ExpiresInSeconds > 0 {
		lifetime = time.Duration(req.ExpiresInSeconds) * time.Second
	}
	expiresAt := time.Now().UTC().Add(lifetime)

	tag, err := h.DB.Exec(r.Context(), `
		INSERT INTO order_claim_tokens
			(merchant_id, merchant_order_ref, customer_email, token_hash, expires_at)
		SELECT $1, $2, $3, $4, $5
		WHERE EXISTS (SELECT 1 FROM merchants WHERE id = $1)
		ON CONFLICT (merchant_id, merchant_order_ref, token_hash)
		DO UPDATE SET expires_at = EXCLUDED.expires_at, consumed_at = NULL`,
		merchantID, orderRef, email, sha256Hex([]byte(token)), expiresAt)
	if err != nil {
		h.Log.Error("register claim token", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not store claim token", "The claim token could not be saved.")
		return
	}
	if tag.RowsAffected() == 0 {
		writeProblem(w, http.StatusBadRequest, "Unknown merchant", "The merchant id does not exist.")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"merchant_order_ref": orderRef,
		"customer_email":     email,
		"expires_at":         expiresAt.Format(time.RFC3339),
	})
}

func (h *Handlers) verifyMerchantSignature(r *http.Request, body []byte) bool {
	secret := strings.TrimSpace(h.MerchantIntegrationSecret)
	if secret == "" {
		return false
	}
	timestamp := r.Header.Get("X-Fractal-Timestamp")
	signature := r.Header.Get("X-Fractal-Signature")
	if timestamp == "" || signature == "" {
		return false
	}
	unix, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	skew := time.Since(time.Unix(unix, 0))
	if skew > 5*time.Minute || skew < -5*time.Minute {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + string(body)))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}
