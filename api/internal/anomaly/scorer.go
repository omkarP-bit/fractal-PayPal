package anomaly

import (
	_ "embed"
	"errors"
	"fmt"
	"time"
)

// Embedded model artifacts, exported by
// `python -m app.anomaly.evaluate --export-dir ../api/internal/anomaly/artifacts`
// from investigator/. See investigator/app/anomaly/README.md (model card).
//
// NOTE: vae.json is intentionally absent — Model C did not beat the best
// baseline by RETAIN_MARGIN on the synthetic benchmark, so it was not
// retained (Phase 11: keep it only if it earns its complexity).
var (
	//go:embed artifacts/baseline.json
	baselineArtifact []byte
	//go:embed artifacts/iforest.json
	iforestArtifact []byte
)

// Model identifiers used for traceability (anomaly_scores.model_id).
const (
	BaselineModelID = "baseline_thresholds"
	IForestModelID  = "isolation_forest"
)

// Score is one traceable anomaly score. Score is in [0,1]; higher = more
// anomalous. Disabled or unscored -> the caller keeps a nil score (null),
// which OPA treats as Phase 10 behavior.
type Score struct {
	Enabled      bool                `json:"enabled"`
	Score        *float64            `json:"score"`
	ModelID      string              `json:"model_id,omitempty"`
	ModelVersion string              `json:"model_version,omitempty"`
	ModelType    string              `json:"model_type,omitempty"`
	Features     map[string]*float64 `json:"features,omitempty"`
	Provenance   map[string]any      `json:"provenance,omitempty"`
	Imputed      []string            `json:"imputed_features,omitempty"`
	CalculatedAt time.Time           `json:"calculated_at"`
}

// Scorer runs the shipped artifacts. It is safe for concurrent use after
// construction.
type Scorer struct {
	enabled  bool
	baseline *BaselineModel
	iforest  *IForestModel
}

// NewScorer parses the embedded artifacts. When enabled is false the scorer
// still loads (so tests can exercise both paths) but Score always returns a
// nil score with Enabled=false — identical behavior to Phase 10.
func NewScorer(enabled bool) (*Scorer, error) {
	baseline, err := ParseBaseline(baselineArtifact)
	if err != nil {
		return nil, err
	}
	forest, err := ParseIForest(iforestArtifact)
	if err != nil {
		return nil, err
	}
	return &Scorer{enabled: enabled, baseline: baseline, iforest: forest}, nil
}

// Enabled reports whether scoring is turned on (FRACTAL_ANOMALY_ENABLED).
func (s *Scorer) Enabled() bool { return s.enabled }

// ModelVersions returns the shipped model versions for traceability.
func (s *Scorer) ModelVersions() (baseline, iforest string) {
	return s.baseline.ModelVersion, s.iforest.ModelVersion
}

// ScoreModel selects which shipped model to run.
type ScoreModel string

// Shipped model selectors.
const (
	UseBaseline ScoreModel = BaselineModelID
	UseIForest  ScoreModel = IForestModelID
)

// Score computes an anomaly score for one merchant's feature vector.
// When the scorer is disabled it returns Score{Enabled:false, Score:nil}
// with no error — the caller must propagate a null score.
func (s *Scorer) Score(model ScoreModel, fv FeatureVector) (Score, error) {
	out := Score{
		Enabled:      s.enabled,
		CalculatedAt: time.Now().UTC(),
		Features:     fv.Values,
		Provenance:   fv.Provenance,
	}
	if !s.enabled {
		return out, nil // score stays nil (null) — Phase 10 equivalence
	}

	var value float64
	var imputed []string
	switch model {
	case UseBaseline, "":
		value, _ = s.baseline.Score(fv.Values)
		out.ModelType = "baseline_thresholds"
		out.ModelVersion = s.baseline.ModelVersion
		out.ModelID = BaselineModelID
	case UseIForest:
		value, imputed = s.iforest.Score(fv.Values)
		out.ModelType = "isolation_forest"
		out.ModelVersion = s.iforest.ModelVersion
		out.ModelID = IForestModelID
		out.Imputed = imputed
	default:
		return Score{}, fmt.Errorf("anomaly: unknown model %q", model)
	}
	if value < 0 || value > 1 {
		return Score{}, fmt.Errorf("anomaly: score out of range: %v", value)
	}
	out.Score = &value
	return out, nil
}

// ErrNoScore reports that no score is available (e.g. extraction failed).
var ErrNoScore = errors.New("anomaly: no score available")
