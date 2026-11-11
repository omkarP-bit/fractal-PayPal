"""Model C (VAE + logistic) tests — skipped entirely when torch is missing."""
import os

import pytest

from app.anomaly import vae
from app.anomaly.features import FEATURE_NAMES

torch = pytest.importorskip("torch", reason="torch not installed; model C is optional")


def _labeled(n=40, seed=0):
    import random

    rng = random.Random(seed)
    rows, labels = [], []
    for i in range(n):
        if i % 4 == 0:
            rows.append({
                "webhook_delivery_latency_p95_s": rng.uniform(100, 300),
                "webhook_retry_count": rng.uniform(10, 30),
                "handler_error_rate": rng.uniform(0.4, 0.9),
                "capture_to_order_update_s": rng.uniform(500, 2000),
                "refund_velocity_per_hour": rng.uniform(8, 30),
            })
            labels.append(1)
        else:
            rows.append({
                "webhook_delivery_latency_p95_s": rng.uniform(0.5, 5),
                "webhook_retry_count": rng.uniform(0, 2),
                "handler_error_rate": rng.uniform(0, 0.1),
                "capture_to_order_update_s": rng.uniform(10, 90),
                "refund_velocity_per_hour": rng.uniform(0, 2),
            })
            labels.append(0)
    return rows, labels


def test_torch_unavailable_error_type_exists():
    # When torch IS installed this just asserts the exception class exists;
    # the raise path is covered by importing behavior in environments w/o torch.
    assert issubclass(vae.TorchUnavailable, RuntimeError)


def test_fit_and_score_in_range():
    rows, labels = _labeled()
    model = vae.fit(rows, labels, model_version="vae-test.1", epochs=30, seed=1)
    assert model.model_version == "vae-test.1"
    for row in (rows[0], rows[3]):
        score, imputed = model.score(row)
        assert 0.0 <= score <= 1.0
        assert imputed == []


def test_missing_feature_imputed_and_reported():
    rows, labels = _labeled()
    model = vae.fit(rows, labels, model_version="v", epochs=30, seed=1)
    probe = dict(rows[0])
    probe["refund_velocity_per_hour"] = None
    score, imputed = model.score(probe)
    assert imputed == ["refund_velocity_per_hour"]
    assert 0.0 <= score <= 1.0


def test_artifact_roundtrip(tmp_path):
    import json

    rows, labels = _labeled()
    model = vae.fit(rows, labels, model_version="vae-rt.1", epochs=20, seed=2)
    path = os.path.join(tmp_path, "vae.json")
    vae.save(path, model)
    reloaded = vae.load(path)
    assert reloaded.model_version == "vae-rt.1"
    assert reloaded.score(rows[0]) == model.score(rows[0])
    with open(path, encoding="utf-8") as fh:
        artifact = json.load(fh)
    assert artifact["model_type"] == "vae_logistic"
    assert artifact["feature_names"] == FEATURE_NAMES
    assert artifact["scaler"]["min"] and artifact["scaler"]["max"]


def test_degenerate_labels_do_not_crash():
    rows, _ = _labeled(n=12)
    model = vae.fit(rows, [0] * len(rows), model_version="v", epochs=10, seed=3)
    score, _ = model.score(rows[0])
    assert 0.0 <= score <= 1.0


def test_retain_margin_is_documented():
    assert vae.RETAIN_MARGIN > 0
    assert vae.MODEL_TYPE == "vae_logistic"
