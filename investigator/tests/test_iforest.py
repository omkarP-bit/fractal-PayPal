"""Isolation Forest (Model B) tests: export parity vs sklearn, scoring, versioning."""
import json
import os

import numpy as np
import pytest
from sklearn.ensemble import IsolationForest

from app.anomaly import iforest
from app.anomaly.features import FEATURE_NAMES


def _training(n_normal=60, n_anomaly=6, seed=0):
    rng = np.random.default_rng(seed)
    rows = []
    labels = []
    for _ in range(n_normal):
        rows.append({
            "webhook_delivery_latency_p95_s": float(rng.uniform(0.5, 5.0)),
            "webhook_retry_count": float(rng.integers(0, 3)),
            "handler_error_rate": float(rng.uniform(0.0, 0.1)),
            "capture_to_order_update_s": float(rng.uniform(10, 90)),
            "refund_velocity_per_hour": float(rng.uniform(0, 2)),
        })
        labels.append(0)
    for _ in range(n_anomaly):
        rows.append({
            "webhook_delivery_latency_p95_s": float(rng.uniform(100, 400)),
            "webhook_retry_count": float(rng.integers(15, 40)),
            "handler_error_rate": float(rng.uniform(0.5, 1.0)),
            "capture_to_order_update_s": float(rng.uniform(600, 3000)),
            "refund_velocity_per_hour": float(rng.uniform(10, 40)),
        })
        labels.append(1)
    return rows, labels


def test_artifact_schema_complete():
    rows, _ = _training()
    model = iforest.fit(rows, model_version="iforest-test.1", n_estimators=10, random_state=1)
    art = model.artifact
    assert art["model_type"] == "isolation_forest"
    assert art["model_version"] == "iforest-test.1"
    assert art["score_range"] == [0.0, 1.0]
    assert art["feature_names"] == FEATURE_NAMES
    assert len(art["trees"]) == 10
    assert art["n_estimators"] == 10
    assert art["c_max"] > 0
    for tree in art["trees"]:
        n = len(tree["children_left"])
        assert len(tree["children_right"]) == n
        assert len(tree["feature"]) == n
        assert len(tree["threshold"]) == n
        assert len(tree["node_depth"]) == n
        assert len(tree["avg_path_len"]) == n
        assert tree["node_depth"][0] == 1  # sklearn depths are 1-based


def test_score_range_and_ordering():
    rows, labels = _training()
    model = iforest.fit(rows, model_version="v", n_estimators=50, random_state=2)
    normal_row = rows[0]
    anomaly_row = rows[-1]
    normal_score, _ = model.score(normal_row)
    anomaly_score, _ = model.score(anomaly_row)
    for score in (normal_score, anomaly_score):
        assert 0.0 < score <= 1.0
    assert anomaly_score > normal_score


def test_exported_walk_matches_sklearn_score_samples():
    """The JSON export must reproduce sklearn's own scoring to float tolerance.

    sklearn score_samples(x) = -2^(-depth/denominator), so our raw score must
    equal -score_samples(x).
    """
    rows, _ = _training(n_normal=40, n_anomaly=4, seed=3)
    model = iforest.fit(rows, model_version="v", n_estimators=25, random_state=7)

    medians = model.impute_medians
    matrix = np.array([
        [r[name] if r.get(name) is not None else medians[name] for name in FEATURE_NAMES]
        for r in rows
    ])
    sk = IsolationForest(
        n_estimators=25, max_samples=model.max_samples, contamination="auto", random_state=7
    )
    sk.fit(matrix)
    expected = -sk.score_samples(matrix)

    for i, row in enumerate(rows):
        got, _ = model.score(row)
        assert got == pytest.approx(expected[i], abs=1e-9), f"row {i}"


def test_missing_features_imputed_with_training_medians():
    rows, _ = _training()
    model = iforest.fit(rows, model_version="v", n_estimators=10, random_state=0)
    all_present = {name: rows[0][name] for name in FEATURE_NAMES}
    with_missing = dict(all_present)
    with_missing["handler_error_rate"] = None
    score_a, imputed_a = model.score(all_present)
    score_b, imputed_b = model.score(with_missing)
    assert imputed_a == []
    assert imputed_b == ["handler_error_rate"]
    # Imputing the training median must not crash and stays in range.
    assert 0.0 < score_b <= 1.0
    assert 0.0 < score_a <= 1.0


def test_model_versioning_recorded():
    rows, _ = _training()
    model = iforest.fit(rows, model_version="iforest-20261110.1", n_estimators=5)
    assert model.model_version == "iforest-20261110.1"
    assert model.artifact["model_version"] == "iforest-20261110.1"


def test_wrong_model_type_rejected():
    with pytest.raises(ValueError):
        iforest.IForestModel({"model_type": "baseline_thresholds"})


def test_artifact_save_load_roundtrip(tmp_path):
    rows, _ = _training()
    model = iforest.fit(rows, model_version="v", n_estimators=8, random_state=4)
    path = os.path.join(tmp_path, "iforest.json")
    iforest.save(path, model)
    reloaded = iforest.load(path)
    probe = rows[0]
    assert reloaded.score(probe) == model.score(probe)
    with open(path, encoding="utf-8") as fh:
        json.load(fh)  # valid JSON


def test_fit_empty_raises():
    with pytest.raises(ValueError):
        iforest.fit([], model_version="v")
