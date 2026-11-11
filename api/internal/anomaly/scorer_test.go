package anomaly

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// goldenCase mirrors one entry of artifacts/golden_scores.json exported by
// evaluate.py (Python-computed scores for the same vectors).
type goldenFile struct {
	DataOrigin string `json:"data_origin"`
	Versions   struct {
		Baseline string `json:"baseline"`
		IForest  string `json:"iforest"`
	} `json:"model_versions"`
	Cases []struct {
		MerchantID    string              `json:"merchant_id"`
		Scenario      string              `json:"scenario"`
		Label         int                 `json:"label"`
		Values        map[string]*float64 `json:"values"`
		BaselineScore float64             `json:"baseline_score"`
		IForestScore  float64             `json:"iforest_score"`
	} `json:"cases"`
}

func loadGolden(t *testing.T) *goldenFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("artifacts", "golden_scores.json"))
	if err != nil {
		t.Fatalf("read golden scores: %v", err)
	}
	var g goldenFile
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatalf("parse golden scores: %v", err)
	}
	if g.DataOrigin != "SYNTHETIC" {
		t.Fatalf("golden scores must be labeled SYNTHETIC, got %q", g.DataOrigin)
	}
	return &g
}

func TestScorerLoadsEmbeddedArtifacts(t *testing.T) {
	s, err := NewScorer(true)
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}
	bv, iv := s.ModelVersions()
	if bv == "" || iv == "" {
		t.Fatalf("empty model versions: baseline=%q iforest=%q", bv, iv)
	}
}

func TestDisabledScorerReturnsNullScore(t *testing.T) {
	s, err := NewScorer(false)
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}
	fv := FeatureVector{
		MerchantID: "MER-1",
		Values:     map[string]*float64{"webhook_retry_count": f64(40)},
	}
	for _, model := range []ScoreModel{UseBaseline, UseIForest} {
		got, err := s.Score(model, fv)
		if err != nil {
			t.Fatalf("Score(%s): %v", model, err)
		}
		if got.Enabled {
			t.Errorf("disabled scorer reported enabled=true")
		}
		if got.Score != nil {
			t.Errorf("disabled scorer produced score %v, want nil (null)", *got.Score)
		}
	}
}

func TestEnabledScorerReturnsScoreInRange(t *testing.T) {
	s, err := NewScorer(true)
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}
	fv := FeatureVector{
		MerchantID: "MER-1",
		Values:     map[string]*float64{"webhook_retry_count": f64(40)},
	}
	for _, model := range []ScoreModel{UseBaseline, UseIForest} {
		got, err := s.Score(model, fv)
		if err != nil {
			t.Fatalf("Score(%s): %v", model, err)
		}
		if !got.Enabled || got.Score == nil {
			t.Fatalf("enabled scorer must produce a score: %+v", got)
		}
		if *got.Score < 0 || *got.Score > 1 {
			t.Errorf("score out of range: %v", *got.Score)
		}
		if got.ModelVersion == "" || got.CalculatedAt.IsZero() {
			t.Errorf("missing traceability: %+v", got)
		}
	}
}

func TestUnknownModelRejected(t *testing.T) {
	s, err := NewScorer(true)
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}
	if _, err := s.Score(ScoreModel("vae_logistic"), FeatureVector{}); err == nil {
		t.Fatal("expected error for unknown model")
	}
}

// TestGoldenParity asserts Go inference matches Python-computed scores from
// evaluate.py to tight tolerance (cross-language parity for both models).
func TestGoldenParity(t *testing.T) {
	g := loadGolden(t)
	s, err := NewScorer(true)
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}
	if len(g.Cases) == 0 {
		t.Fatal("no golden cases")
	}
	for i, c := range g.Cases {
		fv := FeatureVector{MerchantID: c.MerchantID, Values: c.Values}

		base, err := s.Score(UseBaseline, fv)
		if err != nil {
			t.Fatalf("case %d baseline: %v", i, err)
		}
		if math.Abs(*base.Score-c.BaselineScore) > 1e-9 {
			t.Errorf("case %d baseline: go=%v python=%v", i, *base.Score, c.BaselineScore)
		}

		forest, err := s.Score(UseIForest, fv)
		if err != nil {
			t.Fatalf("case %d iforest: %v", i, err)
		}
		if math.Abs(*forest.Score-c.IForestScore) > 1e-9 {
			t.Errorf("case %d iforest: go=%v python=%v", i, *forest.Score, c.IForestScore)
		}
	}
}

func TestFeatureExtractionValidWindow(t *testing.T) {
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	processed := start.Add(10*time.Second + 2*time.Second)

	te := Telemetry{
		MerchantID: "MER-1",
		Window:     Window{Start: start, End: end},
		WebhookEvents: []WebhookEvent{
			{MerchantID: "MER-1", ReceivedAt: start.Add(10 * time.Second), ProcessedAt: &processed, ProcessingStatus: "PROCESSED"},
			{MerchantID: "MER-1", ReceivedAt: start.Add(11 * time.Second), ProcessingStatus: "FAILED"},
		},
		SecurityEvents: []SecurityEvent{
			{MerchantID: "MER-1", EventType: "WEBHOOK_SIGNATURE_INVALID", CreatedAt: start.Add(2 * time.Hour)},
			{MerchantID: "MER-1", EventType: "LOGIN_FAILED", CreatedAt: start.Add(3 * time.Hour)},
		},
		ActionExecutions: []ActionExecution{
			{MerchantID: "MER-1", ActionType: "RECONCILE_MERCHANT_ORDER", Status: "SUCCEEDED", StartedAt: start.Add(4 * time.Hour)},
			{MerchantID: "MER-1", ActionType: "REFUND_PAYMENT", Status: "FAILED", StartedAt: start.Add(5 * time.Hour)},
		},
		PaymentTransactions: []PaymentTransaction{
			{MerchantID: "MER-1", CaptureID: strPtr("C1"), MerchantOrderRef: "O1", LastFetchedAt: start.Add(6 * time.Hour)},
		},
		OrderSnapshots: []OrderSnapshot{
			{MerchantID: "MER-1", MerchantOrderRef: "O1", FetchedAt: start.Add(6*time.Hour + time.Minute)},
		},
	}

	fv, err := Extract(te)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got := *fv.Get("webhook_delivery_latency_p95_s"); got != 2.0 {
		t.Errorf("latency p95 = %v, want 2.0", got)
	}
	// 1 failed webhook + 1 WEBHOOK_* rejection = 2
	if got := *fv.Get("webhook_retry_count"); got != 2 {
		t.Errorf("retry count = %v, want 2", got)
	}
	// 1 failed webhook + 1 failed execution over 2 webhooks + 2 executions
	if got := *fv.Get("handler_error_rate"); math.Abs(got-0.5) > 1e-9 {
		t.Errorf("error rate = %v, want 0.5", got)
	}
	if got := *fv.Get("capture_to_order_update_s"); got != 60 {
		t.Errorf("capture lag = %v, want 60", got)
	}
	if got := *fv.Get("refund_velocity_per_hour"); math.Abs(got-1.0/24.0) > 1e-9 {
		t.Errorf("refund velocity = %v, want 1/24", got)
	}
}

func TestFeatureExtractionEmptyWindow(t *testing.T) {
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	fv, err := Extract(Telemetry{MerchantID: "MER-1", Window: Window{Start: start, End: start.Add(24 * time.Hour)}})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	// count features observe absence -> 0; quantile/ratio -> missing (nil)
	if got := *fv.Get("webhook_retry_count"); got != 0 {
		t.Errorf("retry count = %v, want 0", got)
	}
	if got := *fv.Get("refund_velocity_per_hour"); got != 0 {
		t.Errorf("refund velocity = %v, want 0", got)
	}
	if fv.Get("webhook_delivery_latency_p95_s") != nil {
		t.Error("latency should be missing with no samples")
	}
	if fv.Get("handler_error_rate") != nil {
		t.Error("error rate should be missing with no events")
	}
	if fv.Get("capture_to_order_update_s") != nil {
		t.Error("capture lag should be missing with no pairs")
	}
}

func TestFeatureExtractionRejectsInvalidWindow(t *testing.T) {
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	_, err := Extract(Telemetry{MerchantID: "MER-1", Window: Window{Start: start, End: start}})
	if err != ErrInvalidWindow {
		t.Fatalf("err = %v, want ErrInvalidWindow", err)
	}
	_, err = Extract(Telemetry{MerchantID: "MER-1"})
	if err != ErrInvalidWindow {
		t.Fatalf("err = %v, want ErrInvalidWindow for zero window", err)
	}
}

func TestFeatureExtractionRejectsCrossTenantRows(t *testing.T) {
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	w := Window{Start: start, End: start.Add(24 * time.Hour)}
	cases := []Telemetry{
		{MerchantID: "MER-1", Window: w, WebhookEvents: []WebhookEvent{{MerchantID: "MER-EVIL", ReceivedAt: start}}},
		{MerchantID: "MER-1", Window: w, SecurityEvents: []SecurityEvent{{MerchantID: "MER-EVIL", CreatedAt: start}}},
		{MerchantID: "MER-1", Window: w, ActionExecutions: []ActionExecution{{MerchantID: "MER-EVIL", StartedAt: start}}},
		{MerchantID: "MER-1", Window: w, PaymentTransactions: []PaymentTransaction{{MerchantID: "MER-EVIL", LastFetchedAt: start}}},
		{MerchantID: "MER-1", Window: w, OrderSnapshots: []OrderSnapshot{{MerchantID: "MER-EVIL", FetchedAt: start}}},
	}
	for i, te := range cases {
		if _, err := Extract(te); err != ErrTenantMismatch {
			t.Errorf("case %d: err = %v, want ErrTenantMismatch", i, err)
		}
	}
}

func TestScorerModelVersionsMatchGolden(t *testing.T) {
	g := loadGolden(t)
	s, err := NewScorer(true)
	if err != nil {
		t.Fatalf("NewScorer: %v", err)
	}
	bv, iv := s.ModelVersions()
	if bv != g.Versions.Baseline {
		t.Errorf("baseline version %q != golden %q", bv, g.Versions.Baseline)
	}
	if iv != g.Versions.IForest {
		t.Errorf("iforest version %q != golden %q", iv, g.Versions.IForest)
	}
}

func strPtr(s string) *string { return &s }
