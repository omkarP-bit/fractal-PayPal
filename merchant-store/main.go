package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	store                     = NewStore()
	paypal                    *MPayPal
	fractalURL                string
	merchantID                string
	breakMu                   sync.Mutex
	breakHandler              bool
	merchantIntegrationSecret string
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	clientID := os.Getenv("PAYPAL_CLIENT_ID")
	if clientID == "" {
		clientID = os.Getenv("MERCHANT_PAYPAL_CLIENT_ID")
	}
	clientSecret := os.Getenv("PAYPAL_CLIENT_SECRET")
	if clientSecret == "" {
		clientSecret = os.Getenv("MERCHANT_PAYPAL_CLIENT_SECRET")
	}
	baseURL := os.Getenv("PAYPAL_BASE_URL")
	if baseURL == "" {
		baseURL = os.Getenv("MERCHANT_PAYPAL_BASE_URL")
	}
	if baseURL == "" {
		baseURL = "https://api-m.sandbox.paypal.com"
	}
	paypal = NewMPayPal(clientID, clientSecret, baseURL)
	if paypal.clientID == "" || paypal.clientSecret == "" {
		slog.Error("PayPal credentials are not configured")
	}
	fractalURL = os.Getenv("FRACTAL_API_BASE")
	if fractalURL == "" {
		fractalURL = "http://localhost:8080"
	}
	merchantIntegrationSecret = os.Getenv("MERCHANT_INTEGRATION_SECRET")
	merchantID = os.Getenv("MERCHANT_ID")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleStorefront)
	mux.HandleFunc("POST /api/orders", handleCreateOrder)
	mux.HandleFunc("GET /api/capture/{orderID}", handleCapture)
	mux.HandleFunc("POST /webhooks/paypal", handleMerchantWebhook)
	mux.HandleFunc("GET /orders/{ref}", handleConfirmationPage)
	mux.HandleFunc("GET /api/orders/{ref}", handleOrderJSON)
	mux.HandleFunc("POST /api/orders/{ref}/reconcile", handleReconcile)
	mux.HandleFunc("POST /admin/break", handleAdminBreak)

	addr := os.Getenv("MERCHANT_PORT")
	if addr == "" {
		addr = "8090"
	}
	slog.Info("merchant-store listening", "port", addr)
	_ = http.ListenAndServe(":"+addr, mux)
}

// ---------- storefront ----------

func handleStorefront(w http.ResponseWriter, r *http.Request) {
	breakMu.Lock()
	b := breakHandler
	breakMu.Unlock()
	html := `<html><body>
<h2>Acme Store</h2>
<p>Wireless Mouse — 49.00 USD</p>
<form method="POST" action="/api/orders">
  <label>Email: <input type="email" name="email" value="buyer@example.com" required></label>
  <button type="submit">PayPal Checkout</button>
</form>
<p>Merchant webhook handler: <b>%v</b> — <a href="/admin/break">toggle</a></p>
</body></html>`
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, html, b)
}

// ---------- checkout ----------

func handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	if email == "" {
		email = "buyer@example.com"
	}

	orderID := "ORDER-" + randHex(8)
	claimToken := randHex(16)
	o := store.Create(orderID, 4900, "USD", claimToken)

	publicBase := os.Getenv("MERCHANT_PUBLIC_URL")
	if publicBase == "" {
		publicBase = "http://localhost:8090"
	}
	publicBase = strings.TrimRight(publicBase, "/")
	returnURL := publicBase + "/api/capture/" + orderID
	cancelURL := publicBase + "/?cancelled=1"

	providerOrderID, approveURL, err := paypal.CreateOrder(4900, "USD", orderID, returnURL, cancelURL)
	if err != nil {
		slog.Error("paypal create order", "err", err)
		http.Error(w, `{"error":"checkout failed"}`, http.StatusBadGateway)
		return
	}
	o.ProviderOrderID = providerOrderID

	if err := registerClaimToken(orderID, claimToken, email); err != nil {
		slog.Error("register claim token", "order", orderID, "err", err)
	}

	http.Redirect(w, r, approveURL, http.StatusFound)
}

func registerClaimToken(orderRef, claimToken, email string) error {
	if merchantID == "" {
		return fmt.Errorf("MERCHANT_ID is not set")
	}
	if merchantIntegrationSecret == "" {
		return fmt.Errorf("MERCHANT_INTEGRATION_SECRET is not set")
	}
	body, err := json.Marshal(map[string]any{
		"merchant_id":        merchantID,
		"merchant_order_ref": orderRef,
		"customer_email":     email,
		"token":              claimToken,
		"amount_minor":       4900,
		"currency":           "USD",
	})
	if err != nil {
		return err
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(merchantIntegrationSecret))
	mac.Write([]byte(timestamp + "." + string(body)))
	signature := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequest(http.MethodPost, fractalURL+"/internal/v1/claim-tokens", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Fractal-Timestamp", timestamp)
	req.Header.Set("X-Fractal-Signature", signature)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("fractal returned %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

// After buyer approves on PayPal, they return here. We capture the order.
// PayPal sends the real webhook to this merchant's /webhooks/paypal endpoint
// (registered on the PayPal app). The merchant store updates the order when
// it receives and verifies that webhook.
func handleCapture(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("orderID")
	o, ok := store.Get(orderID)
	if !ok {
		http.NotFound(w, r)
		return
	}
	captureID, _, err := paypal.CaptureOrder(o.ProviderOrderID)
	if err != nil {
		slog.Error("paypal capture", "err", err)
		http.Error(w, `{"error":"capture failed"}`, http.StatusBadGateway)
		return
	}
	o.CaptureID = captureID

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<html><body>
<h2>Captured</h2>
<p>Order %s captured (capture %s). Order status: <b>%s</b>.</p>
<p>PayPal will send a webhook to this store to update the order. If the webhook handler is broken, the order stays PAYMENT_PENDING.</p>
<p><a href="/orders/%s">View confirmation (claim code: %s)</a></p>
</body></html>`, orderID, captureID, o.Status, orderID, o.ClaimToken)
}

// ---------- breakable merchant webhook handler (PayPal -> merchant) ----------

func handleMerchantWebhook(w http.ResponseWriter, r *http.Request) {
	breakMu.Lock()
	broken := breakHandler
	breakMu.Unlock()
	if broken {
		slog.Info("merchant webhook handler BROKEN — returning 500")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"handler failure"}`))
		return
	}
	body, _ := io.ReadAll(r.Body)
	var evt struct {
		ID        string `json:"id"`
		EventType string `json:"event_type"`
		Resource  struct {
			ID string `json:"id"`
		} `json:"resource"`
	}
	if err := json.Unmarshal(body, &evt); err != nil {
		http.Error(w, `{"error":"bad body"}`, http.StatusBadRequest)
		return
	}
	store.mu.Lock()
	for _, o := range store.byRef {
		if o.CaptureID == evt.Resource.ID {
			o.Status = OrderPaid
			o.UpdatedAt = time.Now().UTC()
		}
	}
	store.mu.Unlock()
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"processed"}`))
}

func handleAdminBreak(w http.ResponseWriter, r *http.Request) {
	breakMu.Lock()
	breakHandler = !breakHandler
	b := breakHandler
	breakMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"broken":%v}`, b)
}

// ---------- confirmation page ----------

func handleConfirmationPage(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	o, ok := store.Get(ref)
	if !ok {
		http.NotFound(w, r)
		return
	}
	html := `<html><body>
<h2>Thank you. Order %s</h2>
<p>Status: <b>%s</b> (payment is being confirmed)</p>
<p>Problem with this order?</p>
<p>Your claim code: <b>%s</b></p>
</body></html>`
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, html, ref, o.Status, o.ClaimToken)
}

// ---------- merchant integration contract (Spec C8) ----------

func handleOrderJSON(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	o, ok := store.Get(ref)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"order_ref":"%s","status":"%s","amount_minor":%d,"currency":"%s","updated_at":"%s"}`,
		o.Ref, o.Status, o.AmountMinor, o.Currency, o.UpdatedAt.Format(time.RFC3339))
}

func handleReconcile(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	o, ok := store.Get(ref)
	if !ok {
		http.NotFound(w, r)
		return
	}
	idemKey := r.Header.Get("Idempotency-Key")
	if idemKey == "" {
		http.Error(w, `{"error":"missing Idempotency-Key"}`, http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"unreadable body"}`, http.StatusBadRequest)
		return
	}
	if !verifyHMAC(body, r.Header.Get("X-Fractal-Timestamp"), r.Header.Get("X-Fractal-Signature")) {
		http.Error(w, `{"error":"bad signature"}`, http.StatusUnauthorized)
		return
	}

	var reqBody struct {
		Provider        string `json:"provider"`
		ProviderOrderID string `json:"provider_order_id"`
		CaptureID       string `json:"capture_id"`
		AmountMinor     int64  `json:"amount_minor"`
		Currency        string `json:"currency"`
	}
	if err := json.Unmarshal(body, &reqBody); err != nil {
		http.Error(w, `{"error":"bad body"}`, http.StatusBadRequest)
		return
	}
	if reqBody.AmountMinor != o.AmountMinor || reqBody.Currency != o.Currency {
		w.WriteHeader(http.StatusConflict)
		fmt.Fprintf(w, `{"error":"amount or currency mismatch"}`)
		return
	}
	store.MarkPaid(ref, reqBody.ProviderOrderID, reqBody.CaptureID)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"order_ref":"%s","status":"PAID","reconciled_at":"%s","idempotent_replay":%v}`,
		ref, time.Now().UTC().Format(time.RFC3339), idemKey == "replay-key")
}

func verifyHMAC(body []byte, timestamp, signature string) bool {
	// Spec C8: verify X-Fractal-Signature = hex HMAC-SHA256 of
	// timestamp + "." + raw_body with the per-merchant secret.
	// Reject timestamps more than 5 minutes old.
	if merchantIntegrationSecret == "" {
		slog.Warn("MERCHANT_INTEGRATION_SECRET not set; rejecting request")
		return false
	}
	if timestamp == "" || signature == "" {
		return false
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if time.Now().Unix()-ts > 300 {
		return false
	}

	message := timestamp + "." + string(body)
	mac := hmac.New(sha256.New, []byte(merchantIntegrationSecret))
	mac.Write([]byte(message))
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expected))
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
