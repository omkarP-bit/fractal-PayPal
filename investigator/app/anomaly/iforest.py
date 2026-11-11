"""Model B — Isolation Forest (baseline comparison model).

Trains sklearn.ensemble.IsolationForest on training feature vectors and
exports the fitted trees to a self-contained JSON artifact so the Go runtime
can score without sklearn (api/internal/anomaly/iforest.go implements the
identical walk; parity is asserted in tests against Python reference scores).

Export contents (see artifact schema below):
  * per-tree children_left/children_right/feature/threshold arrays
  * per-node average path length c(n) (precomputed with sklearn's own
    _average_path_length so Go does not re-derive the formula)
  * per-node decision-path depth (tree.compute_node_depths(), 1-based)
  * max_samples used for the normalising constant
  * feature imputation medians for missing features

Scoring (identical to sklearn's _compute_score_samples):
  depth(x)   = sum over trees of (node_depth[leaf] + avg_path_len[leaf] - 1)
  raw(x)     = 2 ** (-depth(x) / (n_estimators * c(max_samples)))
  sklearn score_samples(x) = -raw(x)   ("the lower, the more abnormal")
  We export score = raw(x) in (0, 1], higher = more anomalous, so the OPA
  threshold semantics (score >= 0.8 escalates) hold across models.

Missing features are imputed with training medians (recorded in the
artifact); the set of imputed feature names is returned for provenance.
"""
from __future__ import annotations

import json
from typing import Any

import numpy as np
from sklearn.ensemble import IsolationForest
from sklearn.ensemble._iforest import _average_path_length

from .features import FEATURE_NAMES

MODEL_TYPE = "isolation_forest"


class IForestModel:
    def __init__(self, artifact: dict[str, Any]):
        if artifact.get("model_type") != MODEL_TYPE:
            raise ValueError(f"not a {MODEL_TYPE} artifact: {artifact.get('model_type')!r}")
        self.artifact = artifact
        self.model_version = artifact["model_version"]
        self.feature_names: list[str] = artifact["feature_names"]
        self.impute_medians: dict[str, float] = artifact["impute_medians"]
        self.trees: list[dict[str, Any]] = artifact["trees"]
        self.max_samples: int = artifact["max_samples"]
        self.n_estimators: int = artifact["n_estimators"]

    def vectorize(self, values: dict[str, float | None]) -> tuple[np.ndarray, list[str]]:
        """Impute missing features with training medians; report imputed names."""
        row: list[float] = []
        imputed: list[str] = []
        for name in self.feature_names:
            value = values.get(name)
            if value is None:
                row.append(self.impute_medians[name])
                imputed.append(name)
            else:
                row.append(float(value))
        return np.asarray(row, dtype=float).reshape(1, -1), imputed

    def score(self, values: dict[str, float | None]) -> tuple[float, list[str]]:
        """Return (score in (0,1], higher = more anomalous, imputed features)."""
        x, imputed = self.vectorize(values)
        depth = 0.0
        for tree in self.trees:
            node = 0
            children_left = tree["children_left"]
            while children_left[node] != -1:
                feature = tree["feature"][node]
                threshold = tree["threshold"][node]
                if x[0, feature] <= threshold:
                    node = children_left[node]
                else:
                    node = tree["children_right"][node]
            depth += tree["node_depth"][node] + tree["avg_path_len"][node] - 1.0
        denominator = self.n_estimators * self.c_max
        if denominator == 0:
            return 1.0, imputed
        raw = 2.0 ** (-depth / denominator)
        return float(raw), imputed

    @property
    def c_max(self) -> float:
        return float(self.artifact["c_max"])


def fit(
    vectors: list[dict[str, float | None]],
    *,
    model_version: str,
    n_estimators: int = 100,
    max_samples: int = 256,
    random_state: int = 0,
) -> IForestModel:
    """Fit an IsolationForest (missing -> training median) and export it."""
    if not vectors:
        raise ValueError("no training vectors")
    medians: dict[str, float] = {}
    columns: dict[str, list[float]] = {}
    for name in FEATURE_NAMES:
        observed = [v[name] for v in vectors if v.get(name) is not None]
        if observed:
            med = float(np.median(observed))
        else:
            med = 0.0
        medians[name] = med
        columns[name] = [float(v[name]) if v.get(name) is not None else med for v in vectors]

    matrix = np.column_stack([columns[name] for name in FEATURE_NAMES])
    model = IsolationForest(
        n_estimators=n_estimators,
        max_samples=min(max_samples, len(vectors)),
        contamination="auto",
        random_state=random_state,
    )
    model.fit(matrix)

    trees: list[dict[str, Any]] = []
    for est in model.estimators_:
        tree = est.tree_
        avg_path = _average_path_length(tree.n_node_samples)
        depths = tree.compute_node_depths()
        trees.append(
            {
                "children_left": tree.children_left.astype(int).tolist(),
                "children_right": tree.children_right.astype(int).tolist(),
                "feature": tree.feature.astype(int).tolist(),
                "threshold": [float(t) for t in tree.threshold],
                "node_depth": [int(d) for d in depths],
                "avg_path_len": [float(a) for a in avg_path],
            }
        )

    c_max = float(_average_path_length([model._max_samples])[0])
    artifact = {
        "model_type": MODEL_TYPE,
        "model_version": model_version,
        "score_range": [0.0, 1.0],
        "score_semantics": "2^(-mean_normalised_path_length); higher = more anomalous; "
                           "sklearn score_samples returns the negation",
        "feature_names": FEATURE_NAMES,
        "impute_medians": medians,
        "n_estimators": n_estimators,
        "max_samples": int(model._max_samples),
        "c_max": c_max,
        "random_state": random_state,
        "trees": trees,
        "trained_on": {"n_vectors": len(vectors), "data_origin": "SYNTHETIC-or-labeled-input"},
    }
    return IForestModel(artifact)


def save(path: str, model: IForestModel) -> None:
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(model.artifact, fh, indent=2, sort_keys=True)
        fh.write("\n")


def load(path: str) -> IForestModel:
    with open(path, encoding="utf-8") as fh:
        return IForestModel(json.load(fh))

