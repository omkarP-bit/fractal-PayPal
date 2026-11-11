"""Baseline (Model A) tests: transparent thresholds, ranges, missing data."""
import json
import os

import pytest

from app.anomaly import baseline
from app.anomaly.features import FEATURE_NAMES


def _vectors(n=40, value=10.0):
    return [{name: value for name in FEATURE_NAMES} for _ in range(n)]


def test_fit_produces_documented_fields():
    model = baseline.fit(_vectors(), model_version="baseline-test.1")
    assert model.model_version == "baseline-test.1"
    for name in FEATURE_NAMES:
        spec = model.thresholds[name]
        assert set(spec) == {"q1", "q3", "iqr", "fence", "hard_max"}
        assert spec["hard_max"] == baseline.HARD_MAX[name]
        assert spec["fence"] <= spec["hard_max"]
        assert spec["q1"] <= spec["q3"]


def test_score_zero_at_or_below_fence():
    # Constant training data -> IQR 0 -> fence == q3. Use a value equal to the
    # fence; note handler_error_rate is bounded at 1.0 so 0.5 stays realistic
    # for every feature (fence <= hard_max is enforced in fit).
    model = baseline.fit(_vectors(value=0.5), model_version="v")
    score, exceedances = model.score({name: 0.5 for name in FEATURE_NAMES})
    assert score == 0.0
    assert all(exc == 0.0 for exc in exceedances.values())


def test_score_saturates_at_hard_max():
    model = baseline.fit(_vectors(value=1.0), model_version="v")
    extreme = {name: baseline.HARD_MAX[name] * 2 for name in FEATURE_NAMES}
    score, exceedances = model.score(extreme)
    assert score == 1.0
    assert all(exc == 1.0 for exc in exceedances.values())


def test_score_monotone_in_feature_value():
    model = baseline.fit(_vectors(value=1.0), model_version="v")
    low = model.score({name: 50.0 for name in FEATURE_NAMES})[0]
    high = model.score({name: 500.0 for name in FEATURE_NAMES})[0]
    assert high >= low


def test_missing_features_skipped_not_zero_scored():
    model = baseline.fit(_vectors(value=1.0), model_version="v")
    # all missing -> no observed exceedance -> 0.0 (missing never tightens alone)
    score = model.score({name: None for name in FEATURE_NAMES})[0]
    assert score == 0.0
    # one missing, one extreme -> extreme still reported
    values = {name: None for name in FEATURE_NAMES}
    values["webhook_retry_count"] = baseline.HARD_MAX["webhook_retry_count"] * 2
    score, exceedances = model.score(values)
    assert score == 1.0
    assert exceedances["webhook_retry_count"] == 1.0
    assert exceedances["handler_error_rate"] == 0.0  # missing -> 0 contribution


def test_fit_empty_raises():
    with pytest.raises(ValueError):
        baseline.fit([], model_version="v")


def test_artifact_roundtrip(tmp_path):
    model = baseline.fit(_vectors(), model_version="baseline-roundtrip.1")
    path = os.path.join(tmp_path, "baseline.json")
    baseline.save(path, model)
    reloaded = baseline.load(path)
    assert reloaded.model_version == "baseline-roundtrip.1"
    values = {name: 5.0 for name in FEATURE_NAMES}
    assert reloaded.score(values) == model.score(values)
    with open(path, encoding="utf-8") as fh:
        artifact = json.load(fh)
    assert artifact["model_type"] == "baseline_thresholds"
    assert artifact["score_range"] == [0.0, 1.0]


def test_wrong_model_type_rejected():
    with pytest.raises(ValueError):
        baseline.ThresholdModel.from_artifact({"model_type": "isolation_forest"})


def test_model_versioning_is_distinct_and_recorded():
    a = baseline.fit(_vectors(), model_version="baseline-20261110.1")
    b = baseline.fit(_vectors(value=2.0), model_version="baseline-20261110.2")
    assert a.model_version != b.model_version
    assert a.to_artifact()["model_version"] == "baseline-20261110.1"
