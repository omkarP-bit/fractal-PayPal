package risk

import (
	"time"
)

type Input struct {
	ProviderStatus      string
	OrderStatus         string
	AmountMismatch      bool
	CurrencyMismatch    bool
	EvidenceAgeSeconds  int
	AnomalyScore        *float64
	ProviderUnreachable bool
}

type Output struct {
	Severity     string    // LOW, MEDIUM, HIGH, CRITICAL
	AnomalyScore *float64
	ReasonCodes  []string
}

// Derive derives operational risk from verified facts only. Anomaly score
// can only tighten decisions (never loosen) per the trust boundary.
func Derive(in Input) Output {
	out := Output{
		Severity:     "LOW",
		AnomalyScore: in.AnomalyScore,
	}

	// Critical: provider unreachable during validation — fail closed.
	if in.ProviderUnreachable {
		out.Severity = "CRITICAL"
		out.ReasonCodes = append(out.ReasonCodes, "PROVIDER_UNREACHABLE")
		return out
	}

	// Critical: amount or currency mismatch — blocked by specific rules too.
	if in.AmountMismatch || in.CurrencyMismatch {
		out.Severity = "CRITICAL"
		out.ReasonCodes = append(out.ReasonCodes, "AMOUNT_OR_CURRENCY_MISMATCH")
		return out
	}

	// High: stale evidence.
	if in.EvidenceAgeSeconds > 120 {
		out.Severity = "HIGH"
		out.ReasonCodes = append(out.ReasonCodes, "EVIDENCE_STALE")
	}

	// Medium: provider completed but order pending (the core mismatch scenario).
	if in.ProviderStatus == "COMPLETED" && in.OrderStatus == "PAYMENT_PENDING" {
		if out.Severity == "LOW" {
			out.Severity = "MEDIUM"
		}
		out.ReasonCodes = append(out.ReasonCodes, "PROVIDER_MERCHANT_STATE_MISMATCH")
	}

	// Anomaly score can only tighten: if above threshold, force at least MEDIUM.
	if in.AnomalyScore != nil && *in.AnomalyScore >= 0.8 {
		if out.Severity == "LOW" {
			out.Severity = "MEDIUM"
		}
		out.ReasonCodes = append(out.ReasonCodes, "ANOMALY_SCORE_HIGH")
	}

	return out
}

var _ = time.Now
