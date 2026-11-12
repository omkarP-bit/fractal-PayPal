package assistant

import (
	"strings"
	"testing"
	"time"
)

// --- fingerprinting -------------------------------------------------------

func TestFingerprintGroupsSameRootCauseAcrossCustomers(t *testing.T) {
	base := CaseFacts{
		MerchantID:      "merchant-a",
		ProviderAccountID: "account-a",
		ProviderStatus:  "COMPLETED",
		ProviderAmount:  1000,
		OrderStatus:     "PAYMENT_PENDING",
		OrderAmount:     1000,
		EventType:       "PAYMENT.CAPTURE.COMPLETED",
		ErrorSignature:  "ORDER_UPDATE_MISSING",
		CaseType:        "PROVIDER_MERCHANT_STATE_MISMATCH",
	}
	other := base
	other.CaseID = "case-b"
	other.CustomerID = "customer-b"
	other.MerchantOrderRe = "ORDER-2"
	other.ProviderAmount = 2500
	other.OrderAmount = 2500

	if base.Fingerprint() != other.Fingerprint() {
		t.Fatalf("cases with the same verified cause must share a fingerprint")
	}
}

func TestFingerprintSeparatesDifferentRootCauses(t *testing.T) {
	base := CaseFacts{
		MerchantID:      "merchant-a",
		ProviderAccountID: "account-a",
		ProviderStatus:  "COMPLETED",
		ProviderAmount:  1000,
		OrderStatus:     "PAYMENT_PENDING",
		OrderAmount:     1000,
		EventType:       "PAYMENT.CAPTURE.COMPLETED",
		ErrorSignature:  "ORDER_UPDATE_MISSING",
	}
	// A settled order with a different amount is a different cause: the
	// order update did arrive, only the amount is wrong.
	settled := base
	settled.OrderStatus = "PAID"
	settled.OrderAmount = 999

	if base.Fingerprint() == settled.Fingerprint() {
		t.Fatal("an amount mismatch on a settled order must not share a fingerprint with a missing order update")
	}
}

// TestAmountMismatchStaysInTheSharedIncident checks that one case with a
// different amount still belongs to the shared failure: the policy blocks
// that single case, so it must not be grouped away from the incident.
func TestAmountMismatchStaysInTheSharedIncident(t *testing.T) {
	base := CaseFacts{
		MerchantID:      "merchant-a",
		ProviderAccountID: "account-a",
		ProviderStatus:  "COMPLETED",
		ProviderAmount:  1000,
		OrderStatus:     "PAYMENT_PENDING",
		OrderAmount:     1000,
		EventType:       "PAYMENT.CAPTURE.COMPLETED",
		ErrorSignature:  "ORDER_UPDATE_MISSING",
	}
	mismatch := base
	mismatch.OrderAmount = 1200

	if !mismatch.AmountMismatch() {
		t.Fatal("the fixture must show an amount mismatch")
	}
	if base.Fingerprint() != mismatch.Fingerprint() {
		t.Fatal("a case with an amount mismatch must stay in the incident that shares the outage")
	}
	if err := mismatch.VerifySharedFacts(); err != nil {
		t.Fatalf("the case must still be accepted into the incident: %v", err)
	}
}

func TestFingerprintIsMerchantScoped(t *testing.T) {
	a := CaseFacts{MerchantID: "merchant-a", ProviderAccountID: "acct", ProviderStatus: "COMPLETED", OrderStatus: "PAYMENT_PENDING"}
	b := a
	b.MerchantID = "merchant-b"
	if a.Fingerprint() == b.Fingerprint() {
		t.Fatal("one merchant's fingerprint must never equal another's")
	}
}

// --- verified-fact gate ---------------------------------------------------

func TestVerifySharedFactsRejectsSettledOrder(t *testing.T) {
	facts := CaseFacts{
		MerchantID:     "merchant-a",
		ProviderStatus: "COMPLETED",
		OrderStatus:    "PAID",
		OrderAmount:    1000,
		ProviderAmount: 1000,
	}
	if err := facts.VerifySharedFacts(); err == nil {
		t.Fatal("a settled order must not be accepted into an incident")
	}
}

func TestVerifySharedFactsAcceptsUnconfirmedPaymentWithOpenOrder(t *testing.T) {
	// A dropped webhook leaves the payment unconfirmed for every customer.
	// That is one shared cause, so those cases may group together.
	facts := CaseFacts{
		MerchantID:   "merchant-a",
		ProviderStatus: "PENDING",
		OrderStatus:  "PAYMENT_PENDING",
		OrderAmount:  1000,
	}
	if err := facts.VerifySharedFacts(); err != nil {
		t.Fatalf("expected the case to be accepted: %v", err)
	}
}

func TestVerifySharedFactsAcceptsCapturedPaymentWithMissingOrderUpdate(t *testing.T) {
	facts := CaseFacts{
		MerchantID:     "merchant-a",
		ProviderStatus: "COMPLETED",
		OrderStatus:    "PAYMENT_PENDING",
		OrderAmount:    1000,
		ProviderAmount: 1000,
	}
	if err := facts.VerifySharedFacts(); err != nil {
		t.Fatalf("expected the case to be accepted: %v", err)
	}
}

// --- priority -------------------------------------------------------------

func TestScorePriorityRanksSeverityFirst(t *testing.T) {
	critical := ScorePriority(PriorityInput{Severity: "CRITICAL", AmountMinor: 1000, Age: time.Hour, IncidentSize: 1})
	low := ScorePriority(PriorityInput{Severity: "LOW", AmountMinor: 1000, Age: time.Hour, IncidentSize: 1})
	if critical <= low {
		t.Fatalf("critical severity must outrank low severity (%d vs %d)", critical, low)
	}
}

func TestScorePriorityRanksAmountAboveAge(t *testing.T) {
	big := ScorePriority(PriorityInput{Severity: "MEDIUM", AmountMinor: 500000, Age: time.Hour, IncidentSize: 1})
	small := ScorePriority(PriorityInput{Severity: "MEDIUM", AmountMinor: 100, Age: time.Hour, IncidentSize: 1})
	if big <= small {
		t.Fatalf("a larger amount must outrank a smaller one (%d vs %d)", big, small)
	}
}

func TestScorePriorityRanksAgeAboveNothing(t *testing.T) {
	old := ScorePriority(PriorityInput{Severity: "MEDIUM", AmountMinor: 1000, Age: 48 * time.Hour, IncidentSize: 1})
	fresh := ScorePriority(PriorityInput{Severity: "MEDIUM", AmountMinor: 1000, Age: time.Minute, IncidentSize: 1})
	if old <= fresh {
		t.Fatalf("an older case must outrank a fresh one (%d vs %d)", old, fresh)
	}
}

func TestScorePriorityGrowsWithIncidentSize(t *testing.T) {
	one := ScorePriority(PriorityInput{Severity: "MEDIUM", AmountMinor: 1000, Age: time.Hour, IncidentSize: 1})
	many := ScorePriority(PriorityInput{Severity: "MEDIUM", AmountMinor: 1000, Age: time.Hour, IncidentSize: 6})
	if many <= one {
		t.Fatalf("a larger incident must outrank a single case (%d vs %d)", many, one)
	}
}

// --- guardrails -----------------------------------------------------------

func TestContainsPromiseCatchesOutcomePromises(t *testing.T) {
	bad := []string{
		"We will refund your payment.",
		"You will be refunded within 24 hours.",
		"We guarantee the money is on its way.",
		"This will be resolved by tomorrow.",
	}
	for _, body := range bad {
		if !ContainsPromise(body) {
			t.Errorf("expected a promise to be detected: %q", body)
		}
	}
}

func TestContainsPromiseAllowsFactualStatusUpdates(t *testing.T) {
	good := []string{
		"PayPal confirmed the payment but our order system has not applied the update yet.",
		"We are checking order ORDER-1. To match the payment we need the email used at checkout.",
	}
	for _, body := range good {
		if ContainsPromise(body) {
			t.Errorf("a factual status update must not be flagged: %q", body)
		}
	}
}

func TestCheckDraftBodyRejectsPromise(t *testing.T) {
	if err := CheckDraftBody("We will refund you within 3 days."); err == nil {
		t.Fatal("a draft promising a refund must be rejected")
	}
	if err := CheckDraftBody("We are reviewing your order."); err != nil {
		t.Fatalf("a factual draft must pass: %v", err)
	}
}

func TestContainsMoneyRequestDetectsCustomerRefundAsk(t *testing.T) {
	if !ContainsMoneyRequest("Please refund me, I want my money back.") {
		t.Fatal("expected the customer refund request to be detected")
	}
	if ContainsMoneyRequest("The payment was captured but the order is pending.") {
		t.Fatal("a neutral message must not be read as a money request")
	}
}

// --- draft rendering ------------------------------------------------------

func TestRenderDraftUsesOnlyVerifiedFacts(t *testing.T) {
	body := RenderDraft(DraftStatusUpdate, DraftFacts{
		CustomerName:    "Dana Buyer",
		OrderRef:        "ORDER-9",
		AmountMinor:     4999,
		Currency:        "USD",
		ProviderStatus:  "COMPLETED",
		OrderStatus:     "PAYMENT_PENDING",
		Captured:        true,
	})
	if !strings.Contains(body, "Dana") {
		t.Errorf("expected the customer name in the draft: %q", body)
	}
	if !strings.Contains(body, "ORDER-9") {
		t.Errorf("expected the order reference in the draft: %q", body)
	}
	if !strings.Contains(body, "49.99") {
		t.Errorf("expected the verified amount in the draft: %q", body)
	}
	if ContainsPromise(body) {
		t.Errorf("the rendered draft must not promise anything: %q", body)
	}
}

func TestRenderDraftDoesNotStateRefundWhenCustomerAsksForMoney(t *testing.T) {
	body := RenderDraft(DraftStatusUpdate, DraftFacts{
		CustomerName:   "Dana Buyer",
		OrderRef:       "ORDER-9",
		AmountMinor:    4999,
		Currency:       "USD",
		ProviderStatus: "COMPLETED",
		OrderStatus:    "PAYMENT_PENDING",
		Captured:       true,
	})
	for _, banned := range []string{"refund", "reimburse", "compensat"} {
		if strings.Contains(strings.ToLower(body), banned) {
			t.Errorf("the draft must not mention %q: %q", banned, body)
		}
	}
}

func TestRenderDraftStatusUpdateWhenPaymentNotConfirmed(t *testing.T) {
	body := RenderDraft(DraftStatusUpdate, DraftFacts{
		CustomerName: "Dana Buyer",
		OrderRef:     "ORDER-9",
		Captured:     false,
	})
	if !strings.Contains(body, "verifying") {
		t.Errorf("expected a verification wording: %q", body)
	}
	if ContainsPromise(body) {
		t.Errorf("the rendered draft must not promise anything: %q", body)
	}
}

func TestRenderDraftInformationRequestAsksForOrderEmail(t *testing.T) {
	body := RenderDraft(DraftInformationReq, DraftFacts{
		CustomerName: "Dana Buyer",
		OrderRef:     "ORDER-9",
	})
	if !strings.Contains(strings.ToLower(body), "email address") {
		t.Errorf("expected an information request: %q", body)
	}
	if ContainsPromise(body) {
		t.Errorf("the rendered draft must not promise anything: %q", body)
	}
}

func TestRenderDraftResolutionNoticeOnlyStatesVerifiedOutcome(t *testing.T) {
	body := RenderDraft(DraftResolutionNotice, DraftFacts{
		CustomerName:   "Dana Buyer",
		OrderRef:       "ORDER-9",
		AmountMinor:    4999,
		Currency:       "USD",
		ProviderStatus: "COMPLETED",
		OrderStatus:    "PAID",
		Captured:       true,
	})
	if !strings.Contains(body, "ORDER-9") {
		t.Errorf("expected the order reference: %q", body)
	}
	if ContainsPromise(body) {
		t.Errorf("the rendered draft must not promise anything: %q", body)
	}
}

// --- config ---------------------------------------------------------------

func TestConfigFromEnvDefaultsToQualityGate(t *testing.T) {
	cfg := ConfigFromEnv()
	if cfg.MinCases != 2 {
		t.Errorf("default min cases = %d, want 2", cfg.MinCases)
	}
	if cfg.AutoSend {
		t.Error("auto-send must be off unless the deployment opts in")
	}
}

func TestConfigWithDefaultsFillsZeroValues(t *testing.T) {
	cfg := Config{}.withDefaults()
	if cfg.MinCases != 2 {
		t.Errorf("min cases = %d, want 2", cfg.MinCases)
	}
	if cfg.Window <= 0 {
		t.Error("window must be positive")
	}
}
