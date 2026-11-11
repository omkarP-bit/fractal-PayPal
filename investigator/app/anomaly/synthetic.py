"""SYNTHETIC event generator for Phase 11 anomaly detection.

EVERYTHING THIS MODULE PRODUCES IS SYNTHETIC. Records carry
"data_origin": "SYNTHETIC" and a scenario label. Synthetic data is for
development and evaluation only and must never be presented as real-world
accuracy or mixed into real seed data.

Safety rules:
  * never overwrites an existing file (refuses with FileExistsError);
  * never writes into api/scripts/seed*.sql or any Phase 10 scenario file;
  * output defaults to investigator/data/synthetic/ (git-ignorable).
"""
from __future__ import annotations

import json
import math
import os
import random
from datetime import datetime, timedelta, timezone
from typing import Any

from .features import parse_ts

DATA_ORIGIN = "SYNTHETIC"

# Scenario name -> per-record anomaly injection description (documented ranges).
SCENARIOS: dict[str, str] = {
    "normal": "baseline operational traffic",
    "webhook_failure_burst": "8x webhook failures + slow deliveries (the demo moment)",
    "latency_spike": "webhook delivery latency p95 spikes to minutes",
    "handler_error_spike": "merchant handler errors dominate",
    "capture_order_lag": "captured payments see order updates minutes later",
    "refund_storm": "refund velocity 10x baseline",
}

# Baseline (normal) rate ranges, documented for the model card:
#   webhook events / window:        40..80
#   rejected webhooks / window:     0..2
#   failed webhook processing:      0..2
#   action executions / window:     10..30 (of which 0..1 REFUND_PAYMENT)
#   delivery latency (s):           0.2..3.0 (p95 <= ~5)
#   capture -> order update (s):    10..90
# Anomaly injections multiply or shift these ranges; see each scenario below.


def _iso(dt: datetime) -> str:
    return dt.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def _lognormal(rng: random.Random, median: float, sigma: float) -> float:
    return median * math.exp(rng.gauss(0.0, sigma))


def generate_window(
    merchant_id: str,
    window_start: datetime,
    *,
    scenario: str = "normal",
    seed: int = 0,
    window_hours: float = 24.0,
) -> dict[str, Any]:
    """Generate one merchant's synthetic raw records for one window.

    Returns a dict: metadata + "records" (source -> list of table-shaped rows
    with merchant_id resolved, ready for features.extract_features).
    """
    if scenario not in SCENARIOS:
        raise ValueError(f"unknown scenario {scenario!r}; one of {sorted(SCENARIOS)}")
    rng = random.Random(f"{seed}:{merchant_id}:{scenario}:{_iso(window_start)}")
    start = window_start.astimezone(timezone.utc)
    end = start + timedelta(hours=window_hours)
    n_webhooks = rng.randint(40, 80)
    n_execs = rng.randint(10, 30)

    rejected_webhooks = rng.randint(0, 2)
    failed_webhooks = rng.randint(0, 2)
    failed_execs = rng.randint(0, 1)
    refunds = rng.randint(0, 1)
    latency_median = 1.0
    capture_lag_median = 45.0

    if scenario == "webhook_failure_burst":
        rejected_webhooks = rng.randint(8, 12)   # ~8x baseline high end
        failed_webhooks = rng.randint(8, 14)
        latency_median = 20.0
    elif scenario == "latency_spike":
        latency_median = 180.0
    elif scenario == "handler_error_spike":
        failed_execs = max(failed_execs, int(n_execs * 0.4))
        failed_webhooks = max(failed_webhooks, 6)
    elif scenario == "capture_order_lag":
        capture_lag_median = 900.0
    elif scenario == "refund_storm":
        refunds = rng.randint(10, 18)

    def rand_time() -> datetime:
        offset = rng.uniform(0, window_hours * 3600)
        return start + timedelta(seconds=offset)

    webhook_events: list[dict[str, Any]] = []
    for i in range(n_webhooks):
        received = rand_time()
        latency = _lognormal(rng, latency_median, 0.6)
        failed = i < failed_webhooks
        webhook_events.append(
            {
                "merchant_id": merchant_id,
                "event_type": rng.choice(
                    ["PAYMENT.CAPTURE.COMPLETED", "PAYMENT.CAPTURE.DENIED", "CHECKOUT.ORDER.APPROVED"]
                ),
                "received_at": _iso(received),
                "processed_at": None if failed else _iso(received + timedelta(seconds=latency)),
                "processing_status": "FAILED" if failed else "PROCESSED",
            }
        )

    security_events: list[dict[str, Any]] = []
    for _ in range(rejected_webhooks):
        security_events.append(
            {
                "merchant_id": merchant_id,
                "event_type": rng.choice(
                    ["WEBHOOK_SIGNATURE_INVALID", "WEBHOOK_HEADERS_MISSING", "WEBHOOK_MALFORMED"]
                ),
                "created_at": _iso(rand_time()),
            }
        )

    action_executions: list[dict[str, Any]] = []
    for i in range(n_execs):
        action_type = "REFUND_PAYMENT" if i < refunds else rng.choice(
            ["RECONCILE_MERCHANT_ORDER", "RECONCILE_MERCHANT_ORDER", "CAPTURE_PAYMENT"]
        )
        failed = i < failed_execs
        started = rand_time()
        action_executions.append(
            {
                "merchant_id": merchant_id,
                "action_type": action_type,
                "status": "FAILED" if failed else "SUCCEEDED",
                "started_at": _iso(started),
                "completed_at": _iso(started + timedelta(seconds=rng.uniform(0.2, 4.0))),
            }
        )

    payment_transactions: list[dict[str, Any]] = []
    order_snapshots: list[dict[str, Any]] = []
    n_orders = rng.randint(15, 40)
    for i in range(n_orders):
        ref = f"ORDER-{seed}-{merchant_id}-{i}"
        captured_at = rand_time()
        lag = _lognormal(rng, capture_lag_median, 0.7)
        payment_transactions.append(
            {
                "merchant_id": merchant_id,
                "capture_id": f"CAP-{i}",
                "merchant_order_ref": ref,
                "last_fetched_at": _iso(captured_at),
            }
        )
        order_snapshots.append(
            {
                "merchant_id": merchant_id,
                "merchant_order_ref": ref,
                "fetched_at": _iso(captured_at + timedelta(seconds=lag)),
            }
        )

    return {
        "data_origin": DATA_ORIGIN,
        "synthetic": True,
        "scenario": scenario,
        "scenario_description": SCENARIOS[scenario],
        "merchant_id": merchant_id,
        "window_start": _iso(start),
        "window_end": _iso(end),
        "seed": seed,
        "records": {
            "webhook_events": webhook_events,
            "security_events": security_events,
            "action_executions": action_executions,
            "payment_transactions": payment_transactions,
            "order_snapshots": order_snapshots,
        },
    }


def generate_dataset(
    merchants: int = 12,
    *,
    scenarios: list[str] | None = None,
    seed: int = 7,
    window_hours: float = 24.0,
    start: str = "2026-11-01T00:00:00Z",
) -> dict[str, Any]:
    """Generate a labeled dataset: one window per merchant per scenario."""
    scenarios = scenarios or list(SCENARIOS)
    base = parse_ts(start)
    assert base is not None
    windows: list[dict[str, Any]] = []
    for merchant_idx in range(merchants):
        merchant_id = f"SYN-MER-{merchant_idx:04d}"
        for scenario in scenarios:
            windows.append(
                generate_window(
                    merchant_id,
                    base,
                    scenario=scenario,
                    seed=seed,
                    window_hours=window_hours,
                )
            )
    return {
        "data_origin": DATA_ORIGIN,
        "synthetic": True,
        "disclaimer": (
            "SYNTHETIC data. Not real traffic; metrics computed on this data "
            "are not real-world accuracy."
        ),
        "seed": seed,
        "window_hours": window_hours,
        "scenarios": scenarios,
        "windows": windows,
    }


def write_dataset(path: str, dataset: dict[str, Any], *, exist_ok: bool = False) -> str:
    """Write a dataset as JSON. REFUSES to overwrite an existing file.

    exist_ok=True is intentionally not exposed via the CLI: synthetic data is
    append/replace-safe only through a new path, so prior artifacts are kept.
    """
    if os.path.exists(path) and not exist_ok:
        raise FileExistsError(
            f"refusing to overwrite existing file: {path} (generate a new path instead)"
        )
    directory = os.path.dirname(os.path.abspath(path))
    os.makedirs(directory, exist_ok=True)
    with open(path, "x", encoding="utf-8") as fh:
        json.dump(dataset, fh, indent=2, sort_keys=True)
        fh.write("\n")
    return path
