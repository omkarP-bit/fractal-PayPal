package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ProviderFetcher is the minimal provider capability the evidence collector
// needs: fetch the current provider-side state for an order.
type ProviderFetcher interface {
	GetTransaction(ctx context.Context, providerOrderID string) (*Transaction, error)
}

// Transaction is a normalized view of a provider-side payment.
type Transaction struct {
	Provider         string
	ProviderOrderID  string
	CaptureID        string
	AmountMinor      int64
	Currency         string
	Status           string
	FetchedAt        time.Time
}

type Collector struct {
	db     *pgxpool.Pool
	paypal ProviderFetcher
	merchant MerchantAPI
}

type MerchantAPI interface {
	GetOrder(ctx context.Context, ref string) (OrderSnapshot, error)
}

type OrderSnapshot struct {
	Ref         string
	Status      string
	AmountMinor int64
	Currency    string
	FetchedAt   time.Time
}

func NewCollector(db *pgxpool.Pool, paypal ProviderFetcher, merchant MerchantAPI) *Collector {
	return &Collector{db: db, paypal: paypal, merchant: merchant}
}

// Collect gathers provider, order and webhook evidence for a case and stores
// each row immutably in case_evidence. Returns the evidence IDs.
func (c *Collector) Collect(ctx context.Context, caseID string, providerOrderID string, merchantOrderRef string) ([]string, error) {
	var ids []string

	// 1. Provider transaction evidence (fresh fetch from PayPal).
	tx, err := c.paypal.GetTransaction(ctx, providerOrderID)
	if err != nil {
		return nil, fmt.Errorf("fetch provider: %w", err)
	}
	payload, _ := json.Marshal(tx)
	ids = append(ids, c.storeEvidence(ctx, caseID, "PROVIDER_TRANSACTION", "PAYPAL_API", "paypal:"+providerOrderID, tx.FetchedAt, payload))

	// 2. Merchant order snapshot evidence.
	osnap, err := c.merchant.GetOrder(ctx, merchantOrderRef)
	if err != nil {
		return nil, fmt.Errorf("fetch merchant order: %w", err)
	}
	payload, _ = json.Marshal(osnap)
	ids = append(ids, c.storeEvidence(ctx, caseID, "ORDER_SNAPSHOT", "MERCHANT_API", "merchant:"+merchantOrderRef, osnap.FetchedAt, payload))

	// 3. Webhook history evidence (verified events already stored).
	ids = append(ids, c.storeWebhookEvidence(ctx, caseID, providerOrderID)...)

	return ids, nil
}

func (c *Collector) storeEvidence(ctx context.Context, caseID, evType, source, sourceRef string, fetchedAt time.Time, payload []byte) string {
	var id string
	err := c.db.QueryRow(ctx, `
		INSERT INTO case_evidence (case_id, evidence_type, source_system, source_ref, fetched_at, payload, payload_sha256)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`, caseID, evType, source, sourceRef, fetchedAt, string(payload), sha256Hex(payload)).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}

func (c *Collector) storeWebhookEvidence(ctx context.Context, caseID, providerOrderID string) []string {
	var ids []string
	rows, err := c.db.Query(ctx, `
		SELECT id, event_type, payload, payload_sha256, received_at
		FROM webhook_events WHERE provider = 'PAYPAL' AND resource_id = $1`, providerOrderID)
	if err != nil {
		return ids
	}
	defer rows.Close()
	for rows.Next() {
		var id, eventType, payloadSHA string
		var receivedAt time.Time
		var payload []byte
		if err := rows.Scan(&id, &eventType, &payload, &payloadSHA, &receivedAt); err != nil {
			continue
		}
		// Re-store as case evidence referencing the webhook event.
		var evID string
		_ = c.db.QueryRow(ctx, `
			INSERT INTO case_evidence (case_id, evidence_type, source_system, source_ref, fetched_at, payload, payload_sha256)
			VALUES ($1, 'WEBHOOK_EVENT', 'FRACTAL_DB', $2, $3, $4, $5)
			RETURNING id`, caseID, id, receivedAt, string(payload), payloadSHA).Scan(&evID)
		if evID != "" {
			ids = append(ids, evID)
		}
	}
	return ids
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
