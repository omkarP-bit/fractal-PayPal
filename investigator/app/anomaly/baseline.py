"""Model A — transparent statistical thresholds (baseline).

Method (documented, deterministic):
  For each feature, fit on training vectors:
      fence    = Q3 + 1.5 * IQR            (Tukey upper fence)
      hard_max = documented domain cap     (see HARD_MAX below)
  Score of one vector:
      exceedance_f = clip((value - fence_f) / (hard_max_f - fence_f), 0, 1)
                     for value > fence_f, else 0
      score = max_f exceedance_f            (missing features skipped)
  score in [0, 1]; 0 means "within all fences".

Rationale: the score is a pure function of published numbers, so an operator
can read the artifact and know exactly why a score was produced. Missing
features contribute nothing (a missing signal never tightens a decision
by itself beyond what observed features say).

HARD_MAX values are domain caps chosen for this system (documented ranges,
not learned): they bound the exceedance so one extreme value cannot dwarf
the scale, and they are the point where score saturates at 1.0.
"""
from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Any

from .features import FEATURE_NAMES

MODEL_TYPE = "baseline_thresholds"

# Documented hard caps (same units as features.py). Saturation point of score=1.0.
HARD_MAX: dict[str, float] = {
    "webhook_delivery_latency_p95_s": 900.0,     # 15 minutes p95
    "webhook_retry_count": 50.0,                 # 50 redelivery signals / window
    "handler_error_rate": 1.0,                   # rate is already bounded
    "capture_to_order_update_s": 3600.0,         # 1 hour
    "refund_velocity_per_hour": 50.0,            # 50 refunds / hour
}


@dataclass
class ThresholdModel:
    model_version: str
    thresholds: dict[str, dict[str, float]]  # feature -> {q1,q3,iqr,fence,hard_max}
    trained_on: dict[str, Any]

    def score(self, values: dict[str, float | None]) -> tuple[float, dict[str, float]]:
        """Return (score, per-feature exceedances). Missing features -> 0."""
        exceedances: dict[str, float] = {}
        worst = 0.0
        for name in FEATURE_NAMES:
            value = values.get(name)
            if value is None:
                exceedances[name] = 0.0
                continue
            spec = self.thresholds[name]
            span = spec["hard_max"] - spec["fence"]
            if span <= 0:
                exc = 0.0 if value <= spec["fence"] else 1.0
            else:
                exc = min(1.0, max(0.0, (value - spec["fence"]) / span))
            exceedances[name] = exc
            worst = max(worst, exc)
        return worst, exceedances

    def to_artifact(self) -> dict[str, Any]:
        return {
            "model_type": MODEL_TYPE,
            "model_version": self.model_version,
            "score_range": [0.0, 1.0],
            "method": "max clipped exceedance over Tukey upper fences (Q3+1.5*IQR), "
                      "linear to documented hard_max",
            "hard_max": HARD_MAX,
            "thresholds": self.thresholds,
            "trained_on": self.trained_on,
        }

    @classmethod
    def from_artifact(cls, artifact: dict[str, Any]) -> "ThresholdModel":
        if artifact.get("model_type") != MODEL_TYPE:
            raise ValueError(f"not a {MODEL_TYPE} artifact: {artifact.get('model_type')!r}")
        return cls(
            model_version=artifact["model_version"],
            thresholds=artifact["thresholds"],
            trained_on=artifact.get("trained_on", {}),
        )


def _quantile(sorted_values: list[float], q: float) -> float:
    """Linear-interpolation quantile on a pre-sorted list (no numpy needed)."""
    if not sorted_values:
        raise ValueError("empty")
    if len(sorted_values) == 1:
        return sorted_values[0]
    pos = q * (len(sorted_values) - 1)
    low = int(pos)
    high = min(low + 1, len(sorted_values) - 1)
    frac = pos - low
    return sorted_values[low] * (1 - frac) + sorted_values[high] * frac


def fit(vectors: list[dict[str, float | None]], *, model_version: str) -> ThresholdModel:
    """Fit per-feature Tukey fences on training feature-value dicts."""
    if not vectors:
        raise ValueError("no training vectors")
    thresholds: dict[str, dict[str, float]] = {}
    for name in FEATURE_NAMES:
        observed = sorted(v[name] for v in vectors if v.get(name) is not None)
        if not observed:
            # Feature never observed in training: fence at the hard cap so an
            # observed value can still saturate, never a free pass below data.
            fence = HARD_MAX[name]
            q1 = q3 = iqr = 0.0
        else:
            q1 = _quantile(observed, 0.25)
            q3 = _quantile(observed, 0.75)
            iqr = q3 - q1
            fence = q3 + 1.5 * iqr
            # Never let the fence exceed the hard cap (would make score
            # unreachable and silently disable the feature).
            fence = min(fence, HARD_MAX[name])
        thresholds[name] = {
            "q1": q1,
            "q3": q3,
            "iqr": iqr,
            "fence": fence,
            "hard_max": HARD_MAX[name],
        }
    return ThresholdModel(
        model_version=model_version,
        thresholds=thresholds,
        trained_on={
            "n_vectors": len(vectors),
            "observed_counts": {
                name: sum(1 for v in vectors if v.get(name) is not None)
                for name in FEATURE_NAMES
            },
        },
    )


def save(path: str, model: ThresholdModel) -> None:
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(model.to_artifact(), fh, indent=2, sort_keys=True)
        fh.write("\n")


def load(path: str) -> ThresholdModel:
    with open(path, encoding="utf-8") as fh:
        return ThresholdModel.from_artifact(json.load(fh))
