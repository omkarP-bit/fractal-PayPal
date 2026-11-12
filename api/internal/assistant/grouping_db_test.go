package assistant

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// These tests exercise the assistant against a live PostgreSQL database. They
// create their own merchant, cases and orders in a transaction-scoped schema
// so they never touch demo data. They are skipped when DATABASE_URL is unset.

func testDSN() string {
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		return dsn
	}
	return ""
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testDSN()
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping DB test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// fixture builds a merchant with cases that share one root cause.
type fixture struct {
	merchantID string
	accountID  string
	actorID    string
	cases      []string
	otherCases []string
}

func newFixture(t *testing.T, pool *pgxpool.Pool, suffix string, count int, otherCount int) *fixture {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)

	merchantID := "aaaaaaaa-0000-4000-8000-0000000000" + suffix
	accountID := "bbbbbbbb-0000-4000-8000-0000000000" + suffix
	otherMerchant := "cccccccc-0000-4000-8000-0000000000" + suffix

	mustExec := func(query string, args ...any) {
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			t.Fatalf("fixture exec: %v", err)
		}
	}

	mustExec(`INSERT INTO merchants (id, legal_name, country, currency, onboarding_status)
		VALUES ($1, 'Test Merchant', 'US', 'USD', 'ACTIVE')`, merchantID)
	mustExec(`INSERT INTO merchants (id, legal_name, country, currency, onboarding_status)
		VALUES ($1, 'Other Merchant', 'US', 'USD', 'ACTIVE')`, otherMerchant)
	mustExec(`INSERT INTO provider_accounts (id, merchant_id, provider, environment, external_account_id, credentials_ref)
		VALUES ($1, $2, 'PAYPAL', 'SANDBOX', $3, 'platform')`, accountID, merchantID, "EXT-"+suffix)
	mustExec(`INSERT INTO provider_accounts (id, merchant_id, provider, environment, external_account_id, credentials_ref)
		VALUES ($1, $2, 'PAYPAL', 'SANDBOX', $3, 'platform')`,
		"dddddddd-0000-4000-8000-0000000000"+suffix, otherMerchant, "EXT-OTHER-"+suffix)

	f := &fixture{merchantID: merchantID, accountID: accountID}
	mustExec(`INSERT INTO users (email, display_name, kind, status)
		VALUES ($1, 'Merchant Actor', 'MERCHANT_USER', 'ACTIVE') ON CONFLICT (email) DO NOTHING`, "actor-"+suffix+"@example.com")
	if err := tx.QueryRow(ctx, `SELECT id::text FROM users WHERE email = $1`, "actor-"+suffix+"@example.com").Scan(&f.actorID); err != nil {
		t.Fatalf("read actor id: %v", err)
	}
	mkCase := func(merchant, account, tag string, index int) string {
		customer := "user-" + tag + "-" + suffix
		mustExec(`INSERT INTO users (email, display_name, kind, status)
			VALUES ($1, $2, 'CUSTOMER', 'ACTIVE') ON CONFLICT (email) DO NOTHING`, customer+"@example.com", "Customer "+tag)
		var customerID string
		if err := tx.QueryRow(ctx, `SELECT id::text FROM users WHERE email = $1`, customer+"@example.com").Scan(&customerID); err != nil {
			t.Fatalf("read customer id: %v", err)
		}
		orderRef := "ORDER-" + tag + "-" + suffix + "-" + string(rune('a'+index))
		var paymentID string
		mustExec(`INSERT INTO payment_transactions
			(merchant_id, provider_account_id, provider, provider_order_id, capture_id, amount_minor, currency, provider_status, merchant_order_ref, last_fetched_at)
			VALUES ($1, $2, 'PAYPAL', $3, $4, 1000, 'USD', 'COMPLETED', $5, now())
			RETURNING id::text`, merchant, account, "PO-"+orderRef, "CAP-"+orderRef, orderRef)
		if err := tx.QueryRow(ctx, `SELECT id::text FROM payment_transactions WHERE provider_order_id = $1`, "PO-"+orderRef).Scan(&paymentID); err != nil {
			t.Fatalf("read payment id: %v", err)
		}
		mustExec(`INSERT INTO order_snapshots (merchant_id, merchant_order_ref, amount_minor, currency, order_status, fetched_at, payload_sha256)
			VALUES ($1, $2, 1000, 'USD', 'PAYMENT_PENDING', now(), 'sha')`, merchant, orderRef)
		var caseID string
		mustExec(`INSERT INTO exception_cases
			(merchant_id, customer_id, payment_transaction_id, merchant_order_ref, case_type, status, opened_via, created_at)
			VALUES ($1, $2, $3, $4, 'PROVIDER_MERCHANT_STATE_MISMATCH', 'OPEN', 'CUSTOMER', now())
			RETURNING id::text`, merchant, customerID, paymentID, orderRef)
		if err := tx.QueryRow(ctx, `SELECT id::text FROM exception_cases WHERE merchant_order_ref = $1 AND merchant_id = $2`, orderRef, merchant).Scan(&caseID); err != nil {
			t.Fatalf("read case id: %v", err)
		}
		return caseID
	}

	for i := 0; i < count; i++ {
		f.cases = append(f.cases, mkCase(merchantID, accountID, "A", i))
	}
	for i := 0; i < otherCount; i++ {
		f.otherCases = append(f.otherCases, mkCase(otherMerchant, "dddddddd-0000-4000-8000-0000000000"+suffix, "B", i))
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit fixture: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		exec := func(query string, args ...any) {
			if _, err := pool.Exec(cleanupCtx, query, args...); err != nil {
				t.Logf("cleanup failed for %q: %v", query, err)
			}
		}
		// The audit trail is append-only in production, so the test tears it
		// down by lifting the immutability trigger for the cleanup only.
		exec(`ALTER TABLE audit_events DISABLE TRIGGER trg_audit_immutable`)
		defer exec(`ALTER TABLE audit_events ENABLE TRIGGER trg_audit_immutable`)
		for _, id := range append(append([]string{}, f.cases...), f.otherCases...) {
			exec(`DELETE FROM incident_cases WHERE case_id = $1`, id)
			// case_messages reference the draft, so they go first.
			exec(`DELETE FROM case_messages WHERE case_id = $1`, id)
			exec(`DELETE FROM message_drafts WHERE case_id = $1`, id)
			exec(`DELETE FROM audit_events WHERE case_id = $1`, id)
			exec(`DELETE FROM exception_cases WHERE id = $1`, id)
		}
		for _, mid := range []string{merchantID, otherMerchant} {
			exec(`DELETE FROM audit_events WHERE incident_id IN (SELECT id FROM incidents WHERE merchant_id = $1)`, mid)
			exec(`DELETE FROM incidents WHERE merchant_id = $1`, mid)
			exec(`DELETE FROM order_snapshots WHERE merchant_id = $1`, mid)
			exec(`DELETE FROM payment_transactions WHERE merchant_id = $1`, mid)
			exec(`DELETE FROM provider_accounts WHERE merchant_id = $1`, mid)
			exec(`DELETE FROM merchants WHERE id = $1`, mid)
		}
		exec(`DELETE FROM users WHERE email LIKE $1`, "%"+suffix+"@example.com")
	})
	return f
}

// TestGroupingThresholdRequiresMinCases checks the configurable grouping
// threshold: a single case stays standalone until the threshold is met.
func TestGroupingThresholdRequiresMinCases(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFixture(t, pool, "01", 2, 0)

	svc := New(pool, nil, Config{MinCases: 2, Window: time.Hour})
	incident, err := svc.GroupCase(ctx, f.cases[0])
	if err != nil {
		t.Fatalf("group first case: %v", err)
	}
	if incident == nil {
		t.Fatal("the first case must still create an incident record")
	}
	if incident.CaseCount != 1 {
		t.Fatalf("incident case count = %d, want 1", incident.CaseCount)
	}

	// One case is not a group yet, so it stays out of the inbox.
	inbox, err := svc.ListIncidents(ctx, f.merchantID, false)
	if err != nil {
		t.Fatalf("list incidents: %v", err)
	}
	if len(inbox) != 0 {
		t.Fatalf("inbox shows %d incidents below the threshold, want 0", len(inbox))
	}

	incident, err = svc.GroupCase(ctx, f.cases[1])
	if err != nil {
		t.Fatalf("group second case: %v", err)
	}
	if incident.CaseCount != 2 {
		t.Fatalf("incident case count = %d, want 2 after the threshold is met", incident.CaseCount)
	}
	if incident.ID == "" {
		t.Fatal("the incident must have an id")
	}

	inbox, err = svc.ListIncidents(ctx, f.merchantID, false)
	if err != nil {
		t.Fatalf("list incidents after threshold: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("inbox shows %d incidents, want 1 once the threshold is met", len(inbox))
	}
}

// TestGroupingSeparatesMerchants checks the merchant scoping of grouping.
func TestGroupingSeparatesMerchants(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFixture(t, pool, "02", 1, 1)

	svc := New(pool, nil, Config{MinCases: 1, Window: time.Hour})
	first, err := svc.GroupCase(ctx, f.cases[0])
	if err != nil {
		t.Fatalf("group first case: %v", err)
	}
	second, err := svc.GroupCase(ctx, f.otherCases[0])
	if err != nil {
		t.Fatalf("group other case: %v", err)
	}
	if first == nil || second == nil {
		t.Fatal("both cases must be grouped")
	}
	if first.ID == second.ID {
		t.Fatal("cases of different merchants must never share an incident")
	}
	if first.MerchantID != f.merchantID {
		t.Errorf("incident merchant = %q, want %q", first.MerchantID, f.merchantID)
	}
}

// TestListIncidentsIsMerchantScoped checks that one merchant's inbox never
// contains another merchant's incidents.
func TestListIncidentsIsMerchantScoped(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFixture(t, pool, "03", 1, 1)

	svc := New(pool, nil, Config{MinCases: 1, Window: time.Hour})
	if _, err := svc.GroupCase(ctx, f.cases[0]); err != nil {
		t.Fatalf("group: %v", err)
	}
	if _, err := svc.GroupCase(ctx, f.otherCases[0]); err != nil {
		t.Fatalf("group other: %v", err)
	}

	incidents, err := svc.ListIncidents(ctx, f.merchantID, false)
	if err != nil {
		t.Fatalf("list incidents: %v", err)
	}
	for _, incident := range incidents {
		if incident.MerchantID != f.merchantID {
			t.Fatalf("inbox leaked an incident of merchant %q", incident.MerchantID)
		}
	}

	// The other merchant's incident must not be readable by this merchant.
	otherIncidents, err := svc.ListIncidents(ctx, "cccccccc-0000-4000-8000-000000000002", false)
	if err != nil {
		t.Fatalf("list other incidents: %v", err)
	}
	for _, incident := range otherIncidents {
		if incident.MerchantID != "cccccccc-0000-4000-8000-000000000002" {
			t.Fatalf("inbox leaked an incident of merchant %q", incident.MerchantID)
		}
	}
}

// TestGetIncidentRejectsOtherMerchant checks the per-incident merchant scope.
func TestGetIncidentRejectsOtherMerchant(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFixture(t, pool, "04", 1, 1)

	svc := New(pool, nil, Config{MinCases: 1, Window: time.Hour})
	incident, err := svc.GroupCase(ctx, f.cases[0])
	if err != nil || incident == nil {
		t.Fatalf("group: %v", err)
	}
	if _, err := svc.GetIncident(ctx, "cccccccc-0000-4000-8000-000000000004", incident.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for another merchant, got %v", err)
	}
}

// TestDraftLifecycleApproveAndSend walks a draft from creation to a sent,
// AI-labeled customer message and an audit entry.
func TestDraftLifecycleApproveAndSend(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFixture(t, pool, "05", 1, 0)

	svc := New(pool, nil, Config{MinCases: 1, Window: time.Hour})
	incident, err := svc.GroupCase(ctx, f.cases[0])
	if err != nil || incident == nil {
		t.Fatalf("group: %v", err)
	}
	actorID := f.actorID

	draft, err := svc.DraftCase(ctx, DraftRequest{
		MerchantID: f.merchantID,
		CaseID:     f.cases[0],
		IncidentID: incident.ID,
	})
	if err != nil {
		t.Fatalf("draft case: %v", err)
	}
	if draft.Status != DraftStatusDrafted {
		t.Fatalf("draft status = %q, want DRAFTED", draft.Status)
	}
	if !draft.AIGenerated {
		t.Error("a draft must be marked as AI-generated")
	}
	if draft.Body == "" {
		t.Error("a draft must have a body")
	}
	if ContainsPromise(draft.Body) {
		t.Errorf("the drafted body promises something: %q", draft.Body)
	}

	// An unapproved draft cannot be sent.
	if _, err := svc.SendDraft(ctx, f.merchantID, draft.ID, actorID); err == nil {
		t.Fatal("an unapproved draft must not be sent")
	}

	approved, err := svc.ApproveDraft(ctx, f.merchantID, draft.ID, actorID)
	if err != nil {
		t.Fatalf("approve draft: %v", err)
	}
	if approved.Status != DraftStatusApproved {
		t.Fatalf("draft status = %q, want APPROVED", approved.Status)
	}

	result, err := svc.SendDraft(ctx, f.merchantID, draft.ID, actorID)
	if err != nil {
		t.Fatalf("send draft: %v", err)
	}
	if result.MessageID == "" {
		t.Fatal("sending must record a message")
	}

	var aiGenerated bool
	var authorType string
	if err := pool.QueryRow(ctx, `
		SELECT ai_generated, author_type FROM case_messages WHERE id = $1`, result.MessageID).
		Scan(&aiGenerated, &authorType); err != nil {
		t.Fatalf("read message: %v", err)
	}
	if !aiGenerated {
		t.Error("the sent message must be labeled AI-generated")
	}
	if authorType != "AGENT" {
		t.Errorf("author type = %q, want AGENT", authorType)
	}

	var auditCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_events WHERE case_id = $1 AND event_type = 'AI_MESSAGE_SENT'`, f.cases[0]).
		Scan(&auditCount); err != nil {
		t.Fatalf("count audit events: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("audit events = %d, want 1", auditCount)
	}
}

// TestDraftFlagsCustomerMoneyRequest checks that a customer asking for a
// refund is flagged instead of being answered with a promise.
func TestDraftFlagsCustomerMoneyRequest(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFixture(t, pool, "06", 1, 0)

	if _, err := pool.Exec(ctx, `
		INSERT INTO case_messages (case_id, author_type, visibility, body)
		VALUES ($1, 'CUSTOMER', 'SHARED', 'Please refund me, I want my money back.')`, f.cases[0]); err != nil {
		t.Fatalf("insert customer message: %v", err)
	}

	svc := New(pool, nil, Config{MinCases: 1, Window: time.Hour})
	draft, err := svc.DraftCase(ctx, DraftRequest{MerchantID: f.merchantID, CaseID: f.cases[0]})
	if err != nil {
		t.Fatalf("draft case: %v", err)
	}
	if draft.GuardrailFlag != GuardrailMoneyRequest {
		t.Errorf("guardrail flag = %q, want %q", draft.GuardrailFlag, GuardrailMoneyRequest)
	}
	if ContainsPromise(draft.Body) {
		t.Errorf("the draft must not promise anything: %q", draft.Body)
	}
	for _, banned := range []string{"refund", "money back", "compensat"} {
		if containsFold(draft.Body, banned) {
			t.Errorf("the draft must not mention %q: %q", banned, draft.Body)
		}
	}
}

// TestUpdateDraftRejectsPromise checks the guardrail on a merchant edit.
func TestUpdateDraftRejectsPromise(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFixture(t, pool, "07", 1, 0)

	svc := New(pool, nil, Config{MinCases: 1, Window: time.Hour})
	draft, err := svc.DraftCase(ctx, DraftRequest{MerchantID: f.merchantID, CaseID: f.cases[0]})
	if err != nil {
		t.Fatalf("draft case: %v", err)
	}
	if _, err := svc.UpdateDraft(ctx, f.merchantID, draft.ID, "We will refund you within 24 hours."); err == nil {
		t.Fatal("a merchant edit that promises a refund must be rejected")
	}
	updated, err := svc.UpdateDraft(ctx, f.merchantID, draft.ID, "We are still checking your order.")
	if err != nil {
		t.Fatalf("update draft: %v", err)
	}
	if updated.Body != "We are still checking your order." {
		t.Errorf("body = %q", updated.Body)
	}
}

// TestFanOutKeepsPerCasePolicyChecks checks that removing a case from an
// incident does not change the case's own policy path.
func TestFanOutKeepsPerCasePolicyChecks(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	f := newFixture(t, pool, "08", 2, 0)

	svc := New(pool, nil, Config{MinCases: 1, Window: time.Hour})
	incident, err := svc.GroupCase(ctx, f.cases[0])
	if err != nil || incident == nil {
		t.Fatalf("group: %v", err)
	}
	if _, err := svc.GroupCase(ctx, f.cases[1]); err != nil {
		t.Fatalf("group second: %v", err)
	}

	incident, err = svc.GetIncident(ctx, f.merchantID, incident.ID)
	if err != nil {
		t.Fatalf("get incident: %v", err)
	}
	if incident.CaseCount != 2 {
		t.Fatalf("incident case count = %d, want 2", incident.CaseCount)
	}

	// The merchant corrects the grouping.
	if err := svc.RemoveCase(ctx, f.merchantID, incident.ID, f.cases[1]); err != nil {
		t.Fatalf("remove case: %v", err)
	}
	incident, err = svc.GetIncident(ctx, f.merchantID, incident.ID)
	if err != nil {
		t.Fatalf("get incident after removal: %v", err)
	}
	if incident.CaseCount != 1 {
		t.Fatalf("incident case count = %d, want 1 after removal", incident.CaseCount)
	}

	// The removed case still has its own policy decisions: removing it from
	// the incident must not delete or resolve the case.
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM exception_cases WHERE id = $1`, f.cases[1]).Scan(&status); err != nil {
		t.Fatalf("read case status: %v", err)
	}
	if status == "RESOLVED" {
		t.Fatal("removing a case from an incident must not resolve it")
	}
}

func containsFold(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		indexFold(haystack, needle) >= 0
}

func indexFold(haystack, needle string) int {
	h := []rune(lowercase(haystack))
	n := []rune(lowercase(needle))
	for i := 0; i+len(n) <= len(h); i++ {
		match := true
		for j := range n {
			if h[i+j] != n[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func lowercase(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + 32
		}
	}
	return string(out)
}
