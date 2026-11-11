// Package anomaly implements Phase 11 anomaly scoring: feature extraction
// from existing verified telemetry, inference from exported model artifacts,
// and traceable score results. The signal is tighten-only: a score can only
// make decisions stricter (risk.anomaly_score -> OPA RSK-002); with the
// model disabled or no score available the system behaves as Phase 10.
package anomaly

import (
	"math"
	"sort"
	"time"
)

// FeatureNames is the canonical feature order. It MUST match
// FEATURE_NAMES in investigator/app/anomaly/features.py and the
// feature_names arrays in the exported artifacts.
var FeatureNames = []string{
	"webhook_delivery_latency_p95_s",
	"webhook_retry_count",
	"handler_error_rate",
	"capture_to_order_update_s",
	"refund_velocity_per_hour",
}

// CountFeatures observe absence -> 0 rather than missing (mirrors features.py).
var countFeatures = map[string]bool{
	"webhook_retry_count":      true,
	"refund_velocity_per_hour": true,
}

// Window bounds one scoring aggregation window (tenant-scoped upstream).
type Window struct {
	Start time.Time
	End   time.Time
}

// Valid reports whether the window is well-formed (end after start).
func (w Window) Valid() bool {
	return !w.Start.IsZero() && w.End.After(w.Start)
}

// WebhookEvent mirrors webhook_events rows already joined to a merchant.
type WebhookEvent struct {
	MerchantID       string
	ReceivedAt       time.Time
	ProcessedAt      *time.Time
	ProcessingStatus string // RECEIVED, PROCESSED, FAILED
}

// SecurityEvent mirrors security_events rows already resolved to a merchant.
type SecurityEvent struct {
	MerchantID string
	EventType  string
	CreatedAt  time.Time
}

// ActionExecution mirrors action_executions rows joined via exception_cases.
type ActionExecution struct {
	MerchantID string
	ActionType string
	Status     string // IN_PROGRESS, SUCCEEDED, FAILED
	StartedAt  time.Time
}

// PaymentTransaction mirrors payment_transactions rows (capture observed).
type PaymentTransaction struct {
	MerchantID       string
	CaptureID        *string
	MerchantOrderRef string
	LastFetchedAt    time.Time
}

// OrderSnapshot mirrors order_snapshots rows.
type OrderSnapshot struct {
	MerchantID       string
	MerchantOrderRef string
	FetchedAt        time.Time
}

// Telemetry is the tenant-filtered input to feature extraction. Every row
// must already belong to MerchantID; extract enforces that boundary again.
type Telemetry struct {
	MerchantID          string
	Window              Window
	WebhookEvents       []WebhookEvent
	SecurityEvents      []SecurityEvent
	ActionExecutions    []ActionExecution
	PaymentTransactions []PaymentTransaction
	OrderSnapshots      []OrderSnapshot
}

// FeatureVector holds one merchant's features plus provenance. Nil values
// mean "missing" (JSON null); count features are 0 when nothing matched.
type FeatureVector struct {
	MerchantID  string              `json:"merchant_id"`
	WindowStart string              `json:"window_start"`
	WindowEnd   string              `json:"window_end"`
	Values      map[string]*float64 `json:"values"`
	Provenance  map[string]any      `json:"provenance"`
}

// Get returns the feature value or nil.
func (fv FeatureVector) Get(name string) *float64 { return fv.Values[name] }

func inWindow(t time.Time, w Window) bool {
	return !t.Before(w.Start) && t.Before(w.End)
}

func f64(v float64) *float64 { return &v }

// nearestRankP95 mirrors features.py: sorted[ceil(0.95*n)-1].
func nearestRankP95(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	idx := int(math.Ceil(0.95*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	return f64(sorted[idx])
}

func median(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	return f64(sorted[len(sorted)/2])
}

// Extract computes the five Phase 11 features. Rows whose MerchantID does
// not match t.MerchantID are rejected (strict) so cross-merchant mixing is
// impossible; the SQL layer filters by merchant as well (defence in depth).
func Extract(t Telemetry) (FeatureVector, error) {
	w := t.Window
	if !w.Valid() {
		return FeatureVector{}, ErrInvalidWindow
	}
	for _, e := range t.WebhookEvents {
		if e.MerchantID != t.MerchantID {
			return FeatureVector{}, ErrTenantMismatch
		}
	}
	for _, e := range t.SecurityEvents {
		if e.MerchantID != t.MerchantID {
			return FeatureVector{}, ErrTenantMismatch
		}
	}
	for _, e := range t.ActionExecutions {
		if e.MerchantID != t.MerchantID {
			return FeatureVector{}, ErrTenantMismatch
		}
	}
	for _, p := range t.PaymentTransactions {
		if p.MerchantID != t.MerchantID {
			return FeatureVector{}, ErrTenantMismatch
		}
	}
	for _, s := range t.OrderSnapshots {
		if s.MerchantID != t.MerchantID {
			return FeatureVector{}, ErrTenantMismatch
		}
	}

	// 1. webhook delivery latency p95 (processed events only).
	var latencies []float64
	webhooksInWindow := 0
	failedWebhooks := 0
	for _, e := range t.WebhookEvents {
		if !inWindow(e.ReceivedAt, w) {
			continue
		}
		webhooksInWindow++
		if e.ProcessingStatus == "FAILED" {
			failedWebhooks++
		}
		if e.ProcessedAt != nil {
			delta := e.ProcessedAt.Sub(e.ReceivedAt).Seconds()
			if delta >= 0 {
				latencies = append(latencies, delta)
			}
		}
	}
	latencyP95 := nearestRankP95(latencies)

	// 2. webhook retry count: WEBHOOK_* rejections + FAILED processing rows.
	retryCount := failedWebhooks
	securityInWindow := 0
	for _, e := range t.SecurityEvents {
		if !inWindow(e.CreatedAt, w) {
			continue
		}
		securityInWindow++
		if len(e.EventType) >= 8 && e.EventType[:8] == "WEBHOOK_" {
			retryCount++
		}
	}

	// 3. handler error rate: FAILED / total across webhooks + executions.
	executionsInWindow := 0
	failedExecutions := 0
	refunds := 0
	for _, e := range t.ActionExecutions {
		if !inWindow(e.StartedAt, w) {
			continue
		}
		executionsInWindow++
		if e.Status == "FAILED" {
			failedExecutions++
		}
		if e.ActionType == "REFUND_PAYMENT" {
			refunds++
		}
	}
	denominator := webhooksInWindow + executionsInWindow
	var errorRate *float64
	if denominator > 0 {
		errorRate = f64(float64(failedWebhooks+failedExecutions) / float64(denominator))
	}

	// 4. capture -> order update: first snapshot at/after capture fetch.
	snapshotsByRef := map[string][]time.Time{}
	snapshotsInWindow := 0
	for _, s := range t.OrderSnapshots {
		if !inWindow(s.FetchedAt, w) {
			continue
		}
		snapshotsInWindow++
		snapshotsByRef[s.MerchantOrderRef] = append(snapshotsByRef[s.MerchantOrderRef], s.FetchedAt)
	}
	for ref := range snapshotsByRef {
		times := snapshotsByRef[ref]
		sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
		snapshotsByRef[ref] = times
	}
	var captureLags []float64
	paymentsInWindow := 0
	for _, p := range t.PaymentTransactions {
		if !inWindow(p.LastFetchedAt, w) {
			continue
		}
		paymentsInWindow++
		if p.CaptureID == nil || *p.CaptureID == "" {
			continue
		}
		for _, snap := range snapshotsByRef[p.MerchantOrderRef] {
			if !snap.Before(p.LastFetchedAt) {
				captureLags = append(captureLags, snap.Sub(p.LastFetchedAt).Seconds())
				break
			}
		}
	}
	captureToOrder := median(captureLags)

	// 5. refund velocity per hour.
	windowHours := w.End.Sub(w.Start).Hours()
	var refundVelocity *float64
	if windowHours > 0 {
		refundVelocity = f64(float64(refunds) / windowHours)
	}

	values := map[string]*float64{
		"webhook_delivery_latency_p95_s": latencyP95,
		"webhook_retry_count":            f64(float64(retryCount)),
		"handler_error_rate":             errorRate,
		"capture_to_order_update_s":      captureToOrder,
		"refund_velocity_per_hour":       refundVelocity,
	}

	missing := []string{}
	for _, name := range FeatureNames {
		if values[name] == nil {
			missing = append(missing, name)
		}
	}

	provenance := map[string]any{
		"methods": map[string]string{
			"webhook_delivery_latency_p95_s": "nearest-rank p95 of processed_at-received_at",
			"webhook_retry_count":            "count(WEBHOOK_* security_events) + count(webhook_events FAILED)",
			"handler_error_rate":             "count(FAILED webhook_events + FAILED action_executions) / count(all)",
			"capture_to_order_update_s":      "median of first order_snapshot.fetched_at >= payment.last_fetched_at minus last_fetched_at",
			"refund_velocity_per_hour":       "count(REFUND_PAYMENT action_executions) / window_hours",
		},
		"sources_in_window": map[string]int{
			"webhook_events":       webhooksInWindow,
			"security_events":      securityInWindow,
			"action_executions":    executionsInWindow,
			"payment_transactions": paymentsInWindow,
			"order_snapshots":      snapshotsInWindow,
		},
		"webhook_latency_samples":      len(latencies),
		"capture_lag_samples":          len(captureLags),
		"refund_count":                 refunds,
		"handler_error_numerator":      failedWebhooks + failedExecutions,
		"handler_error_denominator":    denominator,
		"missing_features":             missing,
		"window_hours":                 windowHours,
		"dropped_other_tenant_records": 0,
	}

	return FeatureVector{
		MerchantID:  t.MerchantID,
		WindowStart: w.Start.UTC().Format(time.RFC3339),
		WindowEnd:   w.End.UTC().Format(time.RFC3339),
		Values:      values,
		Provenance:  provenance,
	}, nil
}
