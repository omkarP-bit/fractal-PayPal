package anomaly

import (
	"encoding/json"
	"fmt"
)

// BaselineModel mirrors investigator/app/anomaly/baseline.py (Model A):
// per-feature Tukey upper fence with a linear exceedance to a documented
// hard cap; score = max over features. Missing features contribute 0.
type BaselineModel struct {
	ModelType    string                       `json:"model_type"`
	ModelVersion string                       `json:"model_version"`
	ScoreRange   []float64                    `json:"score_range"`
	Method       string                       `json:"method"`
	HardMax      map[string]float64           `json:"hard_max"`
	Thresholds   map[string]BaselineThreshold `json:"thresholds"`
	TrainedOn    map[string]any               `json:"trained_on"`
}

// BaselineThreshold is one feature's fitted fence.
type BaselineThreshold struct {
	Q1      float64 `json:"q1"`
	Q3      float64 `json:"q3"`
	IQR     float64 `json:"iqr"`
	Fence   float64 `json:"fence"`
	HardMax float64 `json:"hard_max"`
}

// ParseBaseline loads and validates a Model A artifact.
func ParseBaseline(data []byte) (*BaselineModel, error) {
	var m BaselineModel
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("anomaly: parse baseline artifact: %w", err)
	}
	if m.ModelType != "baseline_thresholds" {
		return nil, fmt.Errorf("anomaly: unexpected baseline model_type %q", m.ModelType)
	}
	for _, name := range FeatureNames {
		if _, ok := m.Thresholds[name]; !ok {
			return nil, fmt.Errorf("anomaly: baseline artifact missing threshold for %q", name)
		}
	}
	return &m, nil
}

// Score computes the Model A score in [0,1] plus per-feature exceedances.
// Missing (nil) features contribute 0 — a missing signal never tightens by
// itself. Mirrors ThresholdModel.score in baseline.py exactly.
func (m *BaselineModel) Score(values map[string]*float64) (float64, map[string]float64) {
	exceedances := make(map[string]float64, len(FeatureNames))
	worst := 0.0
	for _, name := range FeatureNames {
		value := values[name]
		spec := m.Thresholds[name]
		if value == nil {
			exceedances[name] = 0
			continue
		}
		span := spec.HardMax - spec.Fence
		var exc float64
		if span <= 0 {
			if *value <= spec.Fence {
				exc = 0
			} else {
				exc = 1
			}
		} else {
			exc = (*value - spec.Fence) / span
			if exc < 0 {
				exc = 0
			}
			if exc > 1 {
				exc = 1
			}
		}
		exceedances[name] = exc
		if exc > worst {
			worst = exc
		}
	}
	return worst, exceedances
}
