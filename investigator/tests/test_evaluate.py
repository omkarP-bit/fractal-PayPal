"""End-to-end evaluation tests: metrics, retention rule, exports."""
import json
import os

from app.anomaly import evaluate as evaluate_mod
from app.anomaly.evaluate import _precision_at_k, _roc_auc, evaluate


def test_roc_auc_perfect_and_inverted():
    labels = [0, 0, 1, 1]
    assert _roc_auc(labels, [0.1, 0.2, 0.8, 0.9]) == 1.0
    assert _roc_auc(labels, [0.9, 0.8, 0.2, 0.1]) == 0.0
    assert _roc_auc([0, 0, 0], [0.1, 0.2, 0.3]) is None  # one class only


def test_roc_auc_with_ties():
    labels = [0, 1]
    assert _roc_auc(labels, [0.5, 0.5]) == 0.5


def test_precision_at_k():
    labels = [1, 0, 1, 0]
    scores = [0.9, 0.8, 0.7, 0.1]
    assert _precision_at_k(labels, scores, 2) == 0.5
    assert _precision_at_k(labels, scores, 0) is None


def test_evaluate_runs_and_reports_synthetic_disclaimer():
    report = evaluate(seed=7, export_dir=None)
    assert report["synthetic"] is True
    assert report["data_origin"] == "SYNTHETIC"
    assert "must never be reported as real-world accuracy" in report["disclaimer"]
    assert report["n_train"] > 0 and report["n_test"] > 0
    assert set(report["models"]) == {
        "baseline_thresholds",
        "isolation_forest",
        "vae_logistic",
    }


def test_vae_retention_rule_applied():
    report = evaluate(seed=7, export_dir=None)
    c_entry = report["models"]["vae_logistic"]
    if c_entry["status"] == "evaluated":
        # Rule: retain only when AUC >= best baseline + margin; the decision
        # string must reflect the boolean.
        retain = c_entry["retain"]
        auc = c_entry["metrics"]["roc_auc"]
        best = c_entry["best_baseline_auc"]
        assert retain == (auc is not None and auc >= best + c_entry["retain_margin_required"])
        # decision strings: "retained: ..." vs "not retained: ..."
        assert c_entry["decision"].startswith("retained") == retain
    else:
        # torch unavailable: no retain key, status explains why.
        assert c_entry["status"] == "torch_unavailable"
        assert c_entry["error"]


def test_export_writes_artifacts_and_goldens(tmp_path):
    export_dir = os.path.join(tmp_path, "artifacts")
    report = evaluate(seed=7, export_dir=export_dir)
    written = set(os.listdir(export_dir))
    assert {"baseline.json", "iforest.json", "golden_scores.json",
            "evaluation_report.json"} <= written
    # VAE only when retained
    if report["models"]["vae_logistic"].get("retain"):
        assert "vae.json" in written
    else:
        assert "vae.json" not in written

    with open(os.path.join(export_dir, "golden_scores.json"), encoding="utf-8") as fh:
        golden = json.load(fh)
    assert golden["data_origin"] == "SYNTHETIC"
    assert golden["model_versions"]["baseline"]
    assert golden["model_versions"]["iforest"]
    for case in golden["cases"]:
        assert set(case["values"]) == set(evaluate_mod.FEATURE_NAMES)
        assert 0.0 <= case["baseline_score"] <= 1.0
        assert 0.0 < case["iforest_score"] <= 1.0
        assert case["label"] in (0, 1)


def test_evaluate_is_deterministic_for_seed():
    a = evaluate(seed=7, export_dir=None)
    b = evaluate(seed=7, export_dir=None)
    assert a["models"]["baseline_thresholds"]["thresholds"] == \
        b["models"]["baseline_thresholds"]["thresholds"]
    assert a["models"]["isolation_forest"]["metrics"] == \
        b["models"]["isolation_forest"]["metrics"]
