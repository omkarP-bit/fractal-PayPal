"""Phase 11 feature extraction from existing verified telemetry.

Feature log (plan Phase 11), each grounded in a table that already exists:

1. webhook_delivery_latency_p95_s
       webhook_events.processed_at - received_at, p95 nearest-rank.
2. webhook_retry_count
       Provider redelivery pressure: rejected webhook deliveries
       (security_events with a WEBHOOK_* type) plus webhook_events rows whose
       processing_status is FAILED. (No attempts column exists yet — see
       webhook/worker.go TODO — so rejected deliveries are the persisted proxy
       for provider retries: any non-2xx makes PayPal redeliver.)
3. handler_error_rate
       (FAILED webhook_events + FAILED action_executions) /
       (all webhook_events + all action_executions) in the window.
4. capture_to_order_update_s
       order_snapshots.fetched_at - payment_transactions.last_fetched_at for
       the same merchant_order_ref, first snapshot at/after capture fetch.
       (payment_transactions has no dedicated capture timestamp; last_fetched_at
       is the persisted observation time of the captured state.)
5. refund_velocity_per_hour
       REFUND_PAYMENT action_executions in the window per window-hour.

Tenant boundary: records must already be resolved to a single merchant
(the Go SQL joins provider_accounts/cases to merchant_id before calling in;
the synthetic generator emits merchant_id directly). Every record is filtered
on record["merchant_id"] == merchant_id here, so records from another merchant
can never contribute (no cross-merchant mixing).

Timestamps are ISO-8601 strings; provenance (window, source counts, missing
features, methods) is returned with every vector.
"""
from __future__ import annotations

import math
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Iterable

FEATURE_NAMES = [
    "webhook_delivery_latency_p95_s",
    "webhook_retry_count",
    "handler_error_rate",
    "capture_to_order_update_s",
    "refund_velocity_per_hour",
]

# Count features are "observed absence" => 0.0 when nothing matched.
# Ratio/quantile features are undefined without data => None (missing).
COUNT_FEATURES = {"webhook_retry_count", "refund_velocity_per_hour"}

SOURCE_KEYS = (
    "webhook_events",
    "security_events",
    "action_executions",
    "payment_transactions",
    "order_snapshots",
)

METHODS = {
    "webhook_delivery_latency_p95_s": "nearest-rank p95 of processed_at-received_at",
    "webhook_retry_count": "count(WEBHOOK_* security_events) + count(webhook_events FAILED)",
    "handler_error_rate": "count(FAILED webhook_events + FAILED action_executions) / count(all)",
    "capture_to_order_update_s": "median of (first order_snapshot.fetched_at >= payment.last_fetched_at) - payment.last_fetched_at",
    "refund_velocity_per_hour": "count(REFUND_PAYMENT action_executions) / window_hours",
}


class TenantMismatchError(ValueError):
    """Raised when a record's merchant_id does not match the requested merchant."""


@dataclass
class FeatureVector:
    merchant_id: str
    window_start: str
    window_end: str
    values: dict[str, float | None]
    provenance: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        return {
            "merchant_id": self.merchant_id,
            "window_start": self.window_start,
            "window_end": self.window_end,
            "values": self.values,
            "provenance": self.provenance,
        }


def parse_ts(value: str | datetime | None) -> datetime | None:
    """Parse an ISO-8601 timestamp (naive values are assumed UTC)."""
    if value is None:
        return None
    if isinstance(value, datetime):
        dt = value
    else:
        text = value.strip()
        if text.endswith("Z"):
            text = text[:-1] + "+00:00"
        try:
            dt = datetime.fromisoformat(text)
        except ValueError:
            return None
    if dt.tzinfo is None:
        dt = dt.replace(tzinfo=timezone.utc)
    return dt


def nearest_rank_p95(values: Iterable[float]) -> float | None:
    """Nearest-rank p95: sorted[ceil(0.95*n)-1]. Deterministic and portable
    (identical implementation exists in Go: internal/anomaly/features.go)."""
    ordered = sorted(values)
    if not ordered:
        return None
    idx = max(0, math.ceil(0.95 * len(ordered)) - 1)
    return float(ordered[idx])


def _in_window(ts: datetime | None, start: datetime, end: datetime) -> bool:
    return ts is not None and start <= ts < end


def _record_merchant(record: dict[str, Any]) -> str | None:
    mid = record.get("merchant_id")
    return str(mid) if mid is not None else None


def extract_features(
    merchant_id: str,
    window_start: str,
    window_end: str,
    data: dict[str, list[dict[str, Any]]],
    *,
    strict_tenant: bool = True,
) -> FeatureVector:
    """Extract the five Phase 11 features for one merchant and one window.

    data maps source names (see SOURCE_KEYS) to record lists. Unknown sources
    are ignored; missing sources behave like empty lists.

    strict_tenant: when True (default) a record whose merchant_id differs from
    merchant_id raises TenantMismatchError; when False such records are
    dropped. Cross-merchant leakage is therefore impossible in either mode.
    """
    start = parse_ts(window_start)
    end = parse_ts(window_end)
    if start is None or end is None or end <= start:
        raise ValueError("window_start/window_end must be valid ISO timestamps with end > start")

    filtered: dict[str, list[dict[str, Any]]] = {}
    dropped_other_tenant = 0
    for source in SOURCE_KEYS:
        rows: list[dict[str, Any]] = []
        for record in data.get(source, []) or []:
            row_merchant = _record_merchant(record)
            if row_merchant != merchant_id:
                if strict_tenant:
                    raise TenantMismatchError(
                        f"{source}: record merchant {row_merchant!r} != requested {merchant_id!r}"
                    )
                dropped_other_tenant += 1
                continue
            rows.append(record)
        filtered[source] = rows

    sources_in_window = {source: 0 for source in SOURCE_KEYS}

    # --- 1. webhook delivery latency p95 --------------------------------
    latencies: list[float] = []
    for rec in filtered["webhook_events"]:
        received = parse_ts(rec.get("received_at"))
        processed = parse_ts(rec.get("processed_at"))
        if not _in_window(received, start, end):
            continue
        sources_in_window["webhook_events"] += 1
        if processed is None:
            continue
        delta = (processed - received).total_seconds()
        if delta >= 0:
            latencies.append(delta)
    latency_p95 = nearest_rank_p95(latencies)

    # --- 2. webhook retry count -----------------------------------------
    retry_count = 0
    for rec in filtered["security_events"]:
        created = parse_ts(rec.get("created_at"))
        if not _in_window(created, start, end):
            continue
        sources_in_window["security_events"] += 1
        if str(rec.get("event_type", "")).startswith("WEBHOOK_"):
            retry_count += 1
    failed_webhooks = 0
    total_webhooks = 0
    for rec in filtered["webhook_events"]:
        # rows already counted for the window above; count status here
        received = parse_ts(rec.get("received_at"))
        if not _in_window(received, start, end):
            continue
        total_webhooks += 1
        if rec.get("processing_status") == "FAILED":
            failed_webhooks += 1
    retry_count += failed_webhooks

    # --- 3. handler error rate ------------------------------------------
    failed_executions = 0
    total_executions = 0
    for rec in filtered["action_executions"]:
        started = parse_ts(rec.get("started_at")) or parse_ts(rec.get("completed_at"))
        if not _in_window(started, start, end):
            continue
        sources_in_window["action_executions"] += 1
        total_executions += 1
        if rec.get("status") == "FAILED":
            failed_executions += 1
    error_denominator = total_webhooks + total_executions
    error_numerator = failed_webhooks + failed_executions
    handler_error_rate = (
        error_numerator / error_denominator if error_denominator > 0 else None
    )

    # --- 4. capture -> order update time --------------------------------
    snapshots: dict[str, list[datetime]] = {}
    for rec in filtered["order_snapshots"]:
        fetched = parse_ts(rec.get("fetched_at"))
        if not _in_window(fetched, start, end):
            continue
        sources_in_window["order_snapshots"] += 1
        ref = str(rec.get("merchant_order_ref", ""))
        snapshots.setdefault(ref, []).append(fetched)
    for ref in snapshots:
        snapshots[ref].sort()

    capture_lags: list[float] = []
    for rec in filtered["payment_transactions"]:
        fetched = parse_ts(rec.get("last_fetched_at"))
        if not _in_window(fetched, start, end):
            continue
        sources_in_window["payment_transactions"] += 1
        if not rec.get("capture_id"):
            continue
        ref = str(rec.get("merchant_order_ref", ""))
        candidates = [s for s in snapshots.get(ref, []) if s >= fetched]
        if candidates:
            capture_lags.append((candidates[0] - fetched).total_seconds())
    capture_to_order = (
        float(sorted(capture_lags)[len(capture_lags) // 2]) if capture_lags else None
    )

    # --- 5. refund velocity ---------------------------------------------
    window_hours = (end - start).total_seconds() / 3600.0
    refund_count = 0
    for rec in filtered["action_executions"]:
        started = parse_ts(rec.get("started_at")) or parse_ts(rec.get("completed_at"))
        if not _in_window(started, start, end):
            continue
        if rec.get("action_type") == "REFUND_PAYMENT":
            refund_count += 1
    refund_velocity = refund_count / window_hours if window_hours > 0 else None

    values: dict[str, float | None] = {
        "webhook_delivery_latency_p95_s": latency_p95,
        "webhook_retry_count": float(retry_count),
        "handler_error_rate": handler_error_rate,
        "capture_to_order_update_s": capture_to_order,
        "refund_velocity_per_hour": refund_velocity,
    }
    missing = [name for name in FEATURE_NAMES if values[name] is None]

    provenance = {
        "methods": METHODS,
        "sources_in_window": sources_in_window,
        "webhook_latency_samples": len(latencies),
        "capture_lag_samples": len(capture_lags),
        "refund_count": refund_count,
        "handler_error_numerator": error_numerator,
        "handler_error_denominator": error_denominator,
        "missing_features": missing,
        "window_hours": window_hours,
        "dropped_other_tenant_records": dropped_other_tenant,
    }
    return FeatureVector(
        merchant_id=merchant_id,
        window_start=window_start,
        window_end=window_end,
        values=values,
        provenance=provenance,
    )
