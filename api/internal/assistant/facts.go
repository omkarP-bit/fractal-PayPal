package assistant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Root cause codes. These describe the *verified* discrepancy, not a guess.
const (
	RootCauseOrderUpdateMissing = "WEBHOOK_ORDER_UPDATE_MISSING"
	RootCauseAmountMismatch     = "AMOUNT_MISMATCH"
	RootCauseCurrencyMismatch   = "CURRENCY_MISMATCH"
	RootCauseDuplicateCharge    = "DUPLICATE_CHARGE"
	RootCausePaymentUnconfirmed = "PAYMENT_UNCONFIRMED"
	RootCauseOther              = "OTHER"
)

// CaseFacts are the verified facts about one exception case. Every value comes
// from the database (provider transactions, merchant order snapshots and
// signature-verified webhook events). Nothing here is model output.
type CaseFacts struct {
	CaseID          string
	CaseNumber      string
	MerchantID      string
	CustomerID      string
	CaseType        string
	Status          string
	MerchantOrderRe string
	CreatedAt       time.Time
	UpdatedAt       time.Time

	ProviderAccountID string
	Provider          string
	ProviderStatus    string
	ProviderAmount    int64
	Currency          string
	ProviderOrderID   string
	CaptureID         string

	OrderStatus  string
	OrderAmount  int64
	OrderCurrency string
	OrderFetchedAt time.Time

	// EventType is the newest verified webhook event type seen for the
	// merchant's provider account.
	EventType string
	// ErrorSignature identifies the failing signal: a failed webhook
	// delivery, a missing order update, or a verified delivery.
	ErrorSignature string
	// EvidenceIDs are the case evidence rows backing these facts.
	EvidenceIDs []string
}

// Captured reports whether the provider confirms the payment.
func (f CaseFacts) Captured() bool {
	switch strings.ToUpper(f.ProviderStatus) {
	case "COMPLETED", "CAPTURED", "PAID":
		return true
	}
	return false
}

// OrderSettled reports whether the merchant order reflects the payment.
func (f CaseFacts) OrderSettled() bool {
	switch strings.ToUpper(f.OrderStatus) {
	case "PAID", "COMPLETED", "FULFILLED", "RECONCILED":
		return true
	}
	return false
}

// AmountMismatch reports whether provider and order amounts disagree. A
// missing provider amount is not a mismatch: it means the provider record is
// unknown, not that the two numbers differ.
func (f CaseFacts) AmountMismatch() bool {
	return f.ProviderAmount > 0 && f.OrderAmount > 0 && f.ProviderAmount != f.OrderAmount
}

// CurrencyMismatch reports whether provider and order currencies disagree.
func (f CaseFacts) CurrencyMismatch() bool {
	return f.OrderCurrency != "" && f.Currency != "" && f.OrderCurrency != f.Currency
}

// RootCause derives the root-cause code from the verified facts.
//
// The order matters: a captured payment whose order was never updated is
// the shared failure (the webhook or order-update path), whatever else is
// wrong with that one case. An amount mismatch on such a case is a
// per-case problem that PolicyGuard catches on its own, so it must not split
// the case out of the incident that shares the outage.
func (f CaseFacts) RootCause() string {
	switch {
	case f.Captured() && !f.OrderSettled():
		return RootCauseOrderUpdateMissing
	case f.AmountMismatch():
		return RootCauseAmountMismatch
	case f.CurrencyMismatch():
		return RootCauseCurrencyMismatch
	case !f.Captured():
		return RootCausePaymentUnconfirmed
	default:
		if strings.EqualFold(f.CaseType, "DUPLICATE_CHARGE") {
			return RootCauseDuplicateCharge
		}
		return RootCauseOther
	}
}

// Fingerprint is the grouping key. Two cases share an incident only when
// their fingerprints match: same provider account, same root cause, same
// failing webhook signal and event type. Amounts and customer identity are
// deliberately excluded so a single incident can span many customers.
func (f CaseFacts) Fingerprint() string {
	parts := []string{
		f.MerchantID,
		f.ProviderAccountID,
		f.RootCause(),
		f.EventType,
		f.ErrorSignature,
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

// VerifySharedFacts is the backend check that gates incident membership.
//
// The assistant proposes a grouping; the Go backend verifies that the case
// really shows the shared discrepancy before accepting it into the incident.
// A case that fails verification is never linked, so a wrong grouping cannot
// inherit another case's evidence.
func (f CaseFacts) VerifySharedFacts() error {
	if f.MerchantID == "" {
		return errors.New("case has no merchant")
	}
	switch f.RootCause() {
	case RootCauseOrderUpdateMissing:
		if !f.Captured() {
			return errors.New("payment is not confirmed by the provider")
		}
		if f.OrderSettled() {
			return errors.New("merchant order is already settled")
		}
		if f.OrderStatus == "" {
			return errors.New("no merchant order snapshot")
		}
	case RootCauseAmountMismatch:
		if !f.AmountMismatch() {
			return errors.New("amounts match")
		}
	case RootCauseCurrencyMismatch:
		if !f.CurrencyMismatch() {
			return errors.New("currencies match")
		}
	case RootCausePaymentUnconfirmed:
		if f.Captured() {
			return errors.New("payment is confirmed")
		}
		if f.OrderSettled() {
			return errors.New("merchant order is already settled")
		}
	default:
		return errors.New("no verified shared discrepancy")
	}
	return nil
}

// FactsLoader loads verified case facts.
type FactsLoader struct {
	db *pgxpool.Pool
}

// NewFactsLoader builds a loader over the database.
func NewFactsLoader(db *pgxpool.Pool) *FactsLoader { return &FactsLoader{db: db} }

const factsQuery = `
SELECT c.id::text, c.case_number, c.merchant_id::text, c.customer_id::text, c.case_type, c.status,
       c.merchant_order_ref, c.created_at, c.updated_at,
       COALESCE(pt.provider_account_id::text, ''), COALESCE(pt.provider, ''),
       COALESCE(pt.provider_status, ''), COALESCE(pt.amount_minor, 0), COALESCE(pt.currency, ''),
       COALESCE(pt.provider_order_id, ''), COALESCE(pt.capture_id, ''),
       COALESCE(os.order_status, ''), COALESCE(os.amount_minor, 0), COALESCE(os.currency, ''),
       COALESCE(os.fetched_at, c.created_at)
FROM exception_cases c
LEFT JOIN payment_transactions pt ON pt.id = c.payment_transaction_id
LEFT JOIN LATERAL (
	SELECT * FROM order_snapshots s
	WHERE s.merchant_id = c.merchant_id AND s.merchant_order_ref = c.merchant_order_ref
	ORDER BY s.fetched_at DESC LIMIT 1
) os ON true
WHERE c.id = $1`

// Load returns the verified facts for one case.
func (l *FactsLoader) Load(ctx context.Context, caseID string) (CaseFacts, error) {
	var f CaseFacts
	err := l.db.QueryRow(ctx, factsQuery, caseID).Scan(
		&f.CaseID, &f.CaseNumber, &f.MerchantID, &f.CustomerID, &f.CaseType, &f.Status,
		&f.MerchantOrderRe, &f.CreatedAt, &f.UpdatedAt,
		&f.ProviderAccountID, &f.Provider,
		&f.ProviderStatus, &f.ProviderAmount, &f.Currency,
		&f.ProviderOrderID, &f.CaptureID,
		&f.OrderStatus, &f.OrderAmount, &f.OrderCurrency, &f.OrderFetchedAt,
	)
	if err != nil {
		return f, err
	}
	if err := l.loadWebhookSignal(ctx, &f); err != nil {
		return f, err
	}
	if f.OrderStatus == "" {
		// The order snapshot is stored as verified evidence when the case is
		// opened. It is the merchant's own view of the order, so it is a
		// valid fact even before the first reconciliation.
		l.loadOrderEvidence(ctx, &f)
	}
	f.EvidenceIDs = l.loadEvidenceIDs(ctx, f.CaseID)
	return f, nil
}

// loadOrderEvidence fills the order facts from the stored ORDER_SNAPSHOT
// evidence when no order_snapshots row exists yet.
func (l *FactsLoader) loadOrderEvidence(ctx context.Context, f *CaseFacts) {
	var payload []byte
	err := l.db.QueryRow(ctx, `
		SELECT payload FROM case_evidence
		WHERE case_id = $1 AND evidence_type = 'ORDER_SNAPSHOT' AND source_system = 'MERCHANT_API'
		ORDER BY fetched_at DESC LIMIT 1`, f.CaseID).Scan(&payload)
	if err != nil {
		return
	}
	var snapshot struct {
		Ref         string `json:"Ref"`
		Status      string `json:"Status"`
		AmountMinor int64  `json:"AmountMinor"`
		Currency    string `json:"Currency"`
	}
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return
	}
	f.OrderStatus = snapshot.Status
	f.OrderAmount = snapshot.AmountMinor
	if snapshot.Currency != "" {
		f.OrderCurrency = snapshot.Currency
	}
}

// LoadMany loads facts for a set of cases, keyed by case id.
func (l *FactsLoader) LoadMany(ctx context.Context, caseIDs []string) (map[string]CaseFacts, error) {
	out := make(map[string]CaseFacts, len(caseIDs))
	for _, id := range caseIDs {
		facts, err := l.Load(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("load facts %s: %w", id, err)
		}
		out[id] = facts
	}
	return out, nil
}

// loadWebhookSignal inspects the merchant's verified webhook history to find
// the failing signal. Only signature-verified, stored events are considered.
func (l *FactsLoader) loadWebhookSignal(ctx context.Context, f *CaseFacts) error {
	if f.ProviderAccountID == "" {
		return nil
	}
	rows, err := l.db.Query(ctx, `
		SELECT event_type, processing_status, signature_verified
		FROM webhook_events
		WHERE provider_account_id = $1
		ORDER BY received_at DESC
		LIMIT 25`, f.ProviderAccountID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var eventType, processingStatus string
		var verified bool
		if err := rows.Scan(&eventType, &processingStatus, &verified); err != nil {
			return err
		}
		if f.EventType == "" {
			f.EventType = eventType
		}
		if !verified {
			f.ErrorSignature = eventType + ":SIGNATURE_FAILED"
			return rows.Err()
		}
		if processingStatus != "PROCESSED" {
			f.ErrorSignature = eventType + ":" + processingStatus
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if f.ErrorSignature == "" {
		switch {
		case f.Captured() && !f.OrderSettled():
			f.ErrorSignature = "ORDER_UPDATE_MISSING"
		case f.Captured():
			f.ErrorSignature = "IN_SYNC"
		default:
			f.ErrorSignature = "PAYMENT_UNCONFIRMED"
		}
	}
	return nil
}

func (l *FactsLoader) loadEvidenceIDs(ctx context.Context, caseID string) []string {
	rows, err := l.db.Query(ctx, `
		SELECT id::text FROM case_evidence WHERE case_id = $1 ORDER BY fetched_at ASC LIMIT 50`, caseID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return ids
		}
		ids = append(ids, id)
	}
	return ids
}

// ErrNoFacts is returned when a case has no verifiable facts at all.
var ErrNoFacts = errors.New("no verified facts for case")

// LoadOrErr loads facts and maps a missing case to a typed error.
func (l *FactsLoader) LoadOrErr(ctx context.Context, caseID string) (CaseFacts, error) {
	facts, err := l.Load(ctx, caseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return facts, fmt.Errorf("%w: %s", ErrNoFacts, caseID)
	}
	return facts, err
}

// Money renders a minor-unit amount for a customer-facing string.
func Money(amountMinor int64, currency string) string {
	if currency == "" {
		currency = "USD"
	}
	return fmt.Sprintf("%s %s", strconv.FormatFloat(float64(amountMinor)/100, 'f', 2, 64), currency)
}
