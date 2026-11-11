"""Phase 11 model evaluation and artifact export.

Benchmarks Model A (thresholds), Model B (Isolation Forest) and — when torch
is available — Model C (VAE + logistic) on a labeled SYNTHETIC holdout, then
writes:

  * evaluation_report.json  (metrics + model card data; SYNTHETIC metrics are
    labeled as such and are NOT real-world accuracy)
  * baseline.json, iforest.json  -> api/internal/anomaly/artifacts/
    (vae.json only if model C beats the best baseline by RETAIN_MARGIN)
  * golden_scores.json -> parity fixtures for the Go scorer tests

Usage:
    python -m app.anomaly.evaluate [--export-dir DIR] [--seed N]
"""
from __future__ import annotations

import argparse
import json
import math
import os
import sys
from datetime import datetime, timezone
from typing import Any

from . import baseline as baseline_mod
from . import iforest as iforest_mod
from . import vae as vae_mod
from .features import FEATURE_NAMES, extract_features
from .synthetic import DATA_ORIGIN, SCENARIOS, generate_dataset, write_dataset

# Scenarios labeled anomalous (1) vs normal (0) for the supervised benchmark.
ANOMALOUS_SCENARIOS = {
    "webhook_failure_burst",
    "latency_spike",
    "handler_error_spike",
    "capture_order_lag",
    "refund_storm",
}


def _roc_auc(labels: list[int], scores: list[float]) -> float | None:
    """Mann-Whitney ROC-AUC with tie handling (pure python, no sklearn dep)."""
    pairs = sorted(zip(scores, labels))
    n_pos = sum(labels)
    n_neg = len(labels) - n_pos
    if n_pos == 0 or n_neg == 0:
        return None
    # rank scores (average ranks for ties)
    ranks = [0.0] * len(pairs)
    i = 0
    while i < len(pairs):
        j = i
        while j + 1 < len(pairs) and pairs[j + 1][0] == pairs[i][0]:
            j += 1
        avg = (i + j) / 2.0 + 1.0  # 1-based average rank
        for k in range(i, j + 1):
            ranks[k] = avg
        i = j + 1
    rank_sum_pos = sum(r for r, (_, label) in zip(ranks, pairs) if label == 1)
    return (rank_sum_pos - n_pos * (n_pos + 1) / 2.0) / (n_pos * n_neg)


def _precision_at_k(labels: list[int], scores: list[float], k: int) -> float | None:
    order = sorted(range(len(scores)), key=lambda i: scores[i], reverse=True)
    top = [labels[i] for i in order[:k]]
    if not top:
        return None
    return sum(top) / len(top)


def build_feature_dataset(seed: int = 7) -> dict[str, Any]:
    """Generate synthetic windows and extract features for each."""
    dataset = generate_dataset(merchants=12, seed=seed)
    rows: list[dict[str, Any]] = []
    for window in dataset["windows"]:
        fv = extract_features(
            window["merchant_id"],
            window["window_start"],
            window["window_end"],
            window["records"],
        )
        label = 1 if window["scenario"] in ANOMALOUS_SCENARIOS else 0
        rows.append(
            {
                "merchant_id": window["merchant_id"],
                "scenario": window["scenario"],
                "window_start": window["window_start"],
                "window_end": window["window_end"],
                "label": label,
                "values": fv.values,
                "provenance": fv.provenance,
            }
        )
    return {"data_origin": DATA_ORIGIN, "rows": rows}


def split(rows: list[dict[str, Any]], holdout_fraction: float = 0.34) -> tuple[list, list]:
    """Deterministic split: every third row goes to holdout (stratified by
    construction since rows are grouped by scenario)."""
    train, test = [], []
    for idx, row in enumerate(rows):
        (test if idx % 3 == 0 else train).append(row)
    if not train or not test:
        raise ValueError("split produced an empty side; add more rows")
    return train, test


def evaluate(seed: int = 7, export_dir: str | None = None, keep_dataset: str | None = None) -> dict[str, Any]:
    stamp = datetime.now(timezone.utc).strftime("%Y%m%d")
    feature_dataset = build_feature_dataset(seed=seed)
    rows = feature_dataset["rows"]
    train_rows, test_rows = split(rows)

    if keep_dataset:
        write_dataset(keep_dataset, generate_dataset(merchants=12, seed=seed))

    train_values = [r["values"] for r in train_rows]
    train_labels = [r["label"] for r in train_rows]
    test_values = [r["values"] for r in test_rows]
    test_labels = [r["label"] for r in test_rows]

    # ---- Model A: transparent thresholds ----
    base_model = baseline_mod.fit(
        train_values, model_version=f"baseline-{stamp}.1"
    )
    base_scores = [base_model.score(v)[0] for v in test_values]

    # ---- Model B: Isolation Forest ----
    forest = iforest_mod.fit(
        train_values, model_version=f"iforest-{stamp}.1", random_state=seed
    )
    forest_scores = [forest.score(v)[0] for v in test_values]

    # ---- Model C: VAE + logistic (optional) ----
    c_scores: list[float] | None = None
    c_status = "not_evaluated"
    c_error: str | None = None
    try:
        c_model = vae_mod.fit(
            train_values, train_labels, model_version=f"vae-{stamp}.1", seed=seed
        )
        c_scores = [c_model.score(v)[0] for v in test_values]
        c_status = "evaluated"
    except vae_mod.TorchUnavailable as exc:
        c_error = str(exc)
        c_status = "torch_unavailable"

    def metrics(scores: list[float]) -> dict[str, Any]:
        return {
            "roc_auc": _roc_auc(test_labels, scores),
            "precision_at_5": _precision_at_k(test_labels, scores, 5),
            "mean_normal_score": _mean(scores, test_labels, 0),
            "mean_anomaly_score": _mean(scores, test_labels, 1),
        }

    report: dict[str, Any] = {
        "data_origin": DATA_ORIGIN,
        "synthetic": True,
        "disclaimer": (
            "SYNTHETIC benchmark. These metrics measure separation on generated "
            "data only and must never be reported as real-world accuracy."
        ),
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "seed": seed,
        "scenarios": SCENARIOS,
        "n_train": len(train_rows),
        "n_test": len(test_rows),
        "n_test_anomalous": int(sum(test_labels)),
        "feature_names": FEATURE_NAMES,
        "models": {
            "baseline_thresholds": {
                "model_version": base_model.model_version,
                "metrics": metrics(base_scores),
                "thresholds": base_model.thresholds,
            },
            "isolation_forest": {
                "model_version": forest.model_version,
                "metrics": metrics(forest_scores),
            },
            "vae_logistic": {
                "status": c_status,
                "error": c_error,
                "metrics": metrics(c_scores) if c_scores is not None else None,
                "retain_margin_required": vae_mod.RETAIN_MARGIN,
            },
        },
    }

    # ---- Retention decision for model C ----
    base_auc = report["models"]["baseline_thresholds"]["metrics"]["roc_auc"]
    forest_auc = report["models"]["isolation_forest"]["metrics"]["roc_auc"]
    best_baseline_auc = max(x for x in (base_auc, forest_auc) if x is not None)
    retain_c = False
    if c_scores is not None:
        c_auc = report["models"]["vae_logistic"]["metrics"]["roc_auc"]
        retain_c = c_auc is not None and c_auc >= best_baseline_auc + vae_mod.RETAIN_MARGIN
        report["models"]["vae_logistic"]["retain"] = retain_c
        report["models"]["vae_logistic"]["best_baseline_auc"] = best_baseline_auc
        report["models"]["vae_logistic"]["decision"] = (
            "retained: AUC >= best baseline + "
            f"{vae_mod.RETAIN_MARGIN}"
            if retain_c
            else f"not retained: AUC gain below required margin {vae_mod.RETAIN_MARGIN}"
        )

    # ---- Export artifacts ----
    exported: list[str] = []
    if export_dir:
        os.makedirs(export_dir, exist_ok=True)
        baseline_mod.save(os.path.join(export_dir, "baseline.json"), base_model)
        iforest_mod.save(os.path.join(export_dir, "iforest.json"), forest)
        exported += ["baseline.json", "iforest.json"]
        if c_scores is not None and retain_c:
            vae_mod.save(os.path.join(export_dir, "vae.json"), c_model)
            exported.append("vae.json")
        # Golden scores for Go parity tests: same vectors, Python-computed.
        golden = {
            "data_origin": DATA_ORIGIN,
            "model_versions": {
                "baseline": base_model.model_version,
                "iforest": forest.model_version,
                "vae": c_model.model_version if (c_scores is not None and retain_c) else None,
            },
            "cases": [
                {
                    "merchant_id": r["merchant_id"],
                    "scenario": r["scenario"],
                    "label": r["label"],
                    "values": r["values"],
                    "baseline_score": base_model.score(r["values"])[0],
                    "iforest_score": forest.score(r["values"])[0],
                }
                for r in test_rows
            ],
        }
        with open(os.path.join(export_dir, "golden_scores.json"), "w", encoding="utf-8") as fh:
            json.dump(golden, fh, indent=2, sort_keys=True)
            fh.write("\n")
        exported.append("golden_scores.json")

        report_path = os.path.join(export_dir, "evaluation_report.json")
        with open(report_path, "w", encoding="utf-8") as fh:
            json.dump(report, fh, indent=2, sort_keys=True)
            fh.write("\n")
        exported.append("evaluation_report.json")
        report["exported"] = exported

    return report


def _mean(scores: list[float], labels: list[int], label: int) -> float | None:
    picked = [s for s, l in zip(scores, labels) if l == label]
    if not picked:
        return None
    return sum(picked) / len(picked)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Evaluate Phase 11 anomaly models (SYNTHETIC)")
    parser.add_argument("--seed", type=int, default=7)
    parser.add_argument(
        "--export-dir",
        default=os.path.join("..", "api", "internal", "anomaly", "artifacts"),
        help="artifact output directory (relative to investigator/)",
    )
    parser.add_argument(
        "--keep-dataset",
        default=None,
        help="optional path for the generated synthetic dataset (refuses overwrite)",
    )
    args = parser.parse_args(argv)

    report = evaluate(seed=args.seed, export_dir=args.export_dir, keep_dataset=args.keep_dataset)

    print("=== Phase 11 evaluation (SYNTHETIC data — not real-world accuracy) ===")
    for name, entry in report["models"].items():
        metrics = entry.get("metrics")
        if metrics is None:
            print(f"{name:22s} status={entry.get('status')} {entry.get('error') or ''}")
            continue
        auc = metrics.get("roc_auc")
        print(
            f"{name:22s} ROC-AUC={auc if auc is None else round(auc, 4)}  "
            f"P@5={metrics.get('precision_at_5')}"
        )
        if "decision" in entry:
            print(f"{'':22s} decision: {entry['decision']}")
    if report.get("exported"):
        print("exported:", ", ".join(report["exported"]))
    return 0


if __name__ == "__main__":
    sys.exit(main())
