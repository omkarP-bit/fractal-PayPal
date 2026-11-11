package anomaly

import (
	"encoding/json"
	"fmt"
	"math"
)

// IForestModel mirrors investigator/app/anomaly/iforest.py (Model B): a
// sklearn IsolationForest exported as self-contained JSON trees so Go can
// score without sklearn.
//
// Scoring formula (identical to sklearn's _compute_score_samples):
//
//	depth(x) = Σ_trees ( node_depth[leaf] + avg_path_len[leaf] − 1 )
//	score(x) = 2^( −depth(x) / (n_estimators · c(max_samples)) )
//
// sklearn's score_samples returns the negation; we export score in (0,1]
// with higher = more anomalous.
type IForestModel struct {
	ModelType      string             `json:"model_type"`
	ModelVersion   string             `json:"model_version"`
	ScoreRange     []float64          `json:"score_range"`
	ScoreSemantics string             `json:"score_semantics"`
	FeatureNames   []string           `json:"feature_names"`
	ImputeMedians  map[string]float64 `json:"impute_medians"`
	NEstimators    int                `json:"n_estimators"`
	MaxSamples     int                `json:"max_samples"`
	CMax           float64            `json:"c_max"`
	RandomState    int                `json:"random_state"`
	Trees          []IForestTree      `json:"trees"`
	TrainedOn      map[string]any     `json:"trained_on"`
}

// IForestTree is one exported isolation tree.
type IForestTree struct {
	ChildrenLeft  []int     `json:"children_left"`
	ChildrenRight []int     `json:"children_right"`
	Feature       []int     `json:"feature"`
	Threshold     []float64 `json:"threshold"`
	NodeDepth     []int     `json:"node_depth"`
	AvgPathLen    []float64 `json:"avg_path_len"`
}

// ParseIForest loads and validates a Model B artifact.
func ParseIForest(data []byte) (*IForestModel, error) {
	var m IForestModel
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("anomaly: parse iforest artifact: %w", err)
	}
	if m.ModelType != "isolation_forest" {
		return nil, fmt.Errorf("anomaly: unexpected iforest model_type %q", m.ModelType)
	}
	if len(m.Trees) == 0 || m.NEstimators != len(m.Trees) {
		return nil, fmt.Errorf("anomaly: iforest artifact has %d trees but n_estimators=%d",
			len(m.Trees), m.NEstimators)
	}
	if m.CMax <= 0 {
		return nil, fmt.Errorf("anomaly: iforest artifact c_max must be > 0")
	}
	for i, name := range FeatureNames {
		if i >= len(m.FeatureNames) || m.FeatureNames[i] != name {
			return nil, fmt.Errorf("anomaly: iforest feature_names out of order: %v", m.FeatureNames)
		}
		if _, ok := m.ImputeMedians[name]; !ok {
			return nil, fmt.Errorf("anomaly: iforest artifact missing impute median for %q", name)
		}
	}
	return &m, nil
}

// Score computes the Model B score in (0,1] and reports which features were
// imputed with training medians (nil inputs).
func (m *IForestModel) Score(values map[string]*float64) (float64, []string) {
	row := make([]float64, len(m.FeatureNames))
	var imputed []string
	for i, name := range m.FeatureNames {
		if v := values[name]; v != nil {
			row[i] = *v
		} else {
			row[i] = m.ImputeMedians[name]
			imputed = append(imputed, name)
		}
	}

	depth := 0.0
	for ti := range m.Trees {
		tree := &m.Trees[ti]
		node := 0
		for {
			left := tree.ChildrenLeft[node]
			if left == -1 {
				break
			}
			if row[tree.Feature[node]] <= tree.Threshold[node] {
				node = left
			} else {
				node = tree.ChildrenRight[node]
			}
		}
		depth += float64(tree.NodeDepth[node]) + tree.AvgPathLen[node] - 1.0
	}

	denominator := float64(m.NEstimators) * m.CMax
	if denominator == 0 {
		return 1.0, imputed
	}
	return math.Pow(2, -depth/denominator), imputed
}
