package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

var (
	store      = NewStore()
	paypal     *MPayPal
	fractalURL string
	breakMu    sync.Mutex
	breakHandler bool
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	paypal = NewMPayPal(os.Getenv("MERCHANT_PAYPAL_CLIENT_ID"), os.Getenv("MERCHANT_PAYPAL_CLIENT_SECRET"), os.Getenv("MERCHANT_PAYPAL_BASE_URL"))
	if paypal == nil || paypal.clientID == "" {
		paypal = NewMPayPal("sb", "sb", "https://api-m.sandbox.paypal.com")
	}
	fractalURL = os.Getenv("FRACTAL_API_BASE")
	if fractalURL == "" {
		fractalURL = "http://localhost:8080"
	}

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
  <button type="submit">PayPal Checkout</button>
</form>
<p>Merchant webhook handler: <b>%v</b> — <a href="/admin/break">toggle</a></p>
</body></html>`
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, html, b)
}

// ---------- checkout ----------

func handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	orderID := "ORDER-" + randHex(8)
	claimToken := randHex(16)
	o := store.Create(orderID, 4900, "USD", claimToken)

	providerOrderID, approveURL, err := paypal.CreateOrder(4900, "USD")
	if err != nil {
		slog.Error("paypal create order", "err", err)
		http.Error(w, `{"error":"checkout failed"}`, http.StatusBadGateway)
		return
	}
	o.ProviderOrderID = providerOrderID

	http.Redirect(w, r, approveURL, http.StatusFound)
}

// After buyer approves on PayPal, they return here. We capture, then simulate
// the PayPal webhook to the merchant's own (breakable) handler.
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

	// Simulate PayPal webhook POST to this merchant's /webhooks/paypal handler.
	payload := map[string]any{
		"id": "evt_" + captureID,
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource": map[string]any{"id": captureID},
	}
	b, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, "http://localhost:8090/webhooks/paypal", nil)
	req.Body = io.NopCloser(io.MultiReader(bytesReaderStr(string(b))))
	req.ContentLength = int64(len(b))
	req.Header.Set("Content-Type", "application/json")
	rec := &captureResponseWriter{header: http.Header{}}
	handleMerchantWebhook(rec, req)
	_ = rec

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<html><body>
<h2>Captured</h2>
<p>Order %s captured (capture %s). Order status: <b>%s</b>.</p>
<p>Webhook handler was <b>%v</b>. If broken, order stays PAYMENT_PENDING.</p>
<p><a href="/orders/%s">View confirmation (claim code: %s)</a></p>
</body></html>`, orderID, captureID, o.Status, breakHandler, orderID, o.ClaimToken)
}

type captureResponseWriter struct {
	header http.Header
}

func (c *captureResponseWriter) Header() http.Header       { return c.header }
func (c *captureResponseWriter) Write([]byte) (int, error) { return 0, nil }
func (c *captureResponseWriter) WriteHeader(int)           {}

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
		Resource  struct{ ID string `json:"id"` } `json:"resource"`
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
	if !validHMAC(r) {
		http.Error(w, `{"error":"bad signature"}`, http.StatusUnauthorized)
		return
	}
	body, _ := io.ReadAll(r.Body)
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

func validHMAC(r *http.Request) bool {
	// Dev-mode: accept any signature. Real impl verifies X-Fractal-Signature HMAC-SHA256.
	return true
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type bytesReader struct {
	s string
	i int
}

func (br *bytesReader) Read(p []byte) (int, error) {
	if br.i >= len(br.s) {
		return 0, io.EOF
	}
	n := copy(p, br.s[br.i:])
	br.i += n
	return n, nil
}

func bytesReaderStr(s string) *bytesReader { return &bytesReader{s: s, i: 0} }
