package cases

import (
	"encoding/json"
	"testing"
	"time"

	"fractal/api/internal/auth"
)

// The anomaly field is admin-only: platform admins and merchant admins.
// Customers and merchant staff must never qualify.
func TestIsAdminAnomalyVisibility(t *testing.T) {
	cases := []struct {
		name   string
		claims auth.Claims
		want   bool
	}{
		{"platform admin", auth.Claims{Kind: "PLATFORM_ADMIN"}, true},
		{"merchant admin", auth.Claims{Kind: "MERCHANT_USER", Role: "MERCHANT_ADMIN"}, true},
		{"merchant staff", auth.Claims{Kind: "MERCHANT_USER", Role: "MERCHANT_STAFF"}, false},
		{"merchant user without role", auth.Claims{Kind: "MERCHANT_USER"}, false},
		{"customer", auth.Claims{Kind: "CUSTOMER"}, false},
	}
	for _, tc := range cases {
		if got := isAdmin(&tc.claims); got != tc.want {
			t.Errorf("%s: isAdmin = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestBuildAnomalyInfoFullPayload(t *testing.T) {
	payload := []byte(`{
		"model_version": "iforest-20261110.1",
		"model_type": "isolation_forest",
		"window_start": "2026-11-09T00:00:00Z",
		"window_end": "2026-11-10T00:00:00Z",
		"values": {"webhook_retry_count": 16, "handler_error_rate": 0.4}
	}`)
	created := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	info, err := buildAnomalyInfo(0.93, "iforest", payload, created)
	if err != nil {
		t.Fatalf("buildAnomalyInfo: %v", err)
	}
	if info.Score != 0.93 || info.ModelID != "iforest" {
		t.Errorf("score/model mismatch: %+v", info)
	}
	if info.ModelVersion != "iforest-20261110.1" {
		t.Errorf("model_version = %q", info.ModelVersion)
	}
	if info.WindowStart != "2026-11-09T00:00:00Z" || info.WindowEnd != "2026-11-10T00:00:00Z" {
		t.Errorf("window mismatch: %q..%q", info.WindowStart, info.WindowEnd)
	}
	if info.CalculatedAt != "2026-11-10T08:00:00Z" {
		t.Errorf("calculated_at = %q", info.CalculatedAt)
	}
	if info.Threshold != anomalyApprovalThreshold {
		t.Errorf("threshold = %v, want %v", info.Threshold, anomalyApprovalThreshold)
	}
	if info.Features["webhook_retry_count"] == nil || *info.Features["webhook_retry_count"] != 16 {
		t.Errorf("features not decoded: %v", info.Features)
	}
}

func TestBuildAnomalyInfoLegacyPayload(t *testing.T) {
	info, err := buildAnomalyInfo(0.5, "baseline", nil, time.Now())
	if err != nil {
		t.Fatalf("empty payload must not fail: %v", err)
	}
	if info.ModelVersion != "" || info.Features != nil {
		t.Errorf("legacy payload should degrade to score+model: %+v", info)
	}
}

func TestBuildAnomalyInfoMalformedPayloadErrors(t *testing.T) {
	if _, err := buildAnomalyInfo(0.5, "baseline", []byte("{not json"), time.Now()); err == nil {
		t.Fatal("malformed features must return an error (handler omits the field)")
	}
}

// The anomaly field must serialize as omitted when nil so non-admin and
// disabled responses are byte-identical to Phase 10 (no new key).
func TestCaseJSONOmitsNilAnomaly(t *testing.T) {
	raw, err := json.Marshal(Case{ID: "c1", CaseNumber: "FR-1"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(raw); containsAnomalyKey(got) {
		t.Errorf("nil anomaly must be omitted, got %s", raw)
	}

	withScore := Case{
		ID:      "c1",
		Anomaly: &AnomalyInfo{Score: 0.9, ModelID: "iforest", Threshold: 0.8},
	}
	raw, err = json.Marshal(withScore)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !containsAnomalyKey(string(raw)) {
		t.Errorf("set anomaly must be present, got %s", raw)
	}
}

func containsAnomalyKey(s string) bool {
	return len(s) > 0 && (json.Valid([]byte(s)) && hasAnomalyField(s))
}

func hasAnomalyField(s string) bool {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return false
	}
	_, ok := m["anomaly"]
	return ok
}
