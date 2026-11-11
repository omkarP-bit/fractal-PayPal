"""Feature extraction tests: valid windows, missing data, tenant isolation."""
import pytest

from app.anomaly.features import (
    FEATURE_NAMES,
    TenantMismatchError,
    extract_features,
    nearest_rank_p95,
    parse_ts,
)

WINDOW_START = "2026-11-01T00:00:00Z"
WINDOW_END = "2026-11-02T00:00:00Z"


def _webhook(merchant="MER-1", received="2026-11-01T10:00:00Z", processed="2026-11-01T10:00:02Z",
             status="PROCESSED"):
    return {
        "merchant_id": merchant,
        "received_at": received,
        "processed_at": processed,
        "processing_status": status,
    }


def test_parse_ts_z_suffix_and_naive():
    assert parse_ts("2026-11-01T10:00:00Z") is not None
    assert parse_ts("2026-11-01T10:00:00") is not None  # naive -> UTC
    assert parse_ts("not-a-time") is None
    assert parse_ts(None) is None


def test_nearest_rank_p95():
    assert nearest_rank_p95([]) is None
    assert nearest_rank_p95([1.0]) == 1.0
    values = list(range(1, 101))  # 100 samples: ceil(0.95*100)=95 -> index 94
    assert nearest_rank_p95(values) == 95.0


def test_valid_window_extracts_all_features():
    data = {
        "webhook_events": [
            _webhook(received=f"2026-11-01T10:00:{i:02d}Z", processed=f"2026-11-01T10:00:{i + 2:02d}Z")
            for i in range(10)
        ],
        "security_events": [
            {"merchant_id": "MER-1", "event_type": "WEBHOOK_SIGNATURE_INVALID",
             "created_at": "2026-11-01T11:00:00Z"},
            {"merchant_id": "MER-1", "event_type": "SECURITY_SOMETHING_ELSE",
             "created_at": "2026-11-01T11:00:00Z"},
        ],
        "action_executions": [
            {"merchant_id": "MER-1", "action_type": "RECONCILE_MERCHANT_ORDER",
             "status": "SUCCEEDED", "started_at": "2026-11-01T12:00:00Z"},
            {"merchant_id": "MER-1", "action_type": "REFUND_PAYMENT",
             "status": "FAILED", "started_at": "2026-11-01T13:00:00Z"},
        ],
        "payment_transactions": [
            {"merchant_id": "MER-1", "capture_id": "C1", "merchant_order_ref": "O1",
             "last_fetched_at": "2026-11-01T14:00:00Z"},
        ],
        "order_snapshots": [
            {"merchant_id": "MER-1", "merchant_order_ref": "O1",
             "fetched_at": "2026-11-01T14:01:00Z"},
        ],
    }
    fv = extract_features("MER-1", WINDOW_START, WINDOW_END, data)
    assert set(fv.values) == set(FEATURE_NAMES)
    assert fv.values["webhook_delivery_latency_p95_s"] == 2.0
    assert fv.values["webhook_retry_count"] == 1.0  # only the WEBHOOK_* rejection
    # 1 failed execution + 1 failed webhook? -> no FAILED webhooks here:
    # numerator=1 failed exec, denominator=10 webhooks + 2 execs
    assert fv.values["handler_error_rate"] == pytest.approx(1 / 12)
    assert fv.values["capture_to_order_update_s"] == 60.0
    assert fv.values["refund_velocity_per_hour"] == pytest.approx(1 / 24)
    assert fv.provenance["missing_features"] == []
    assert fv.provenance["dropped_other_tenant_records"] == 0


def test_missing_sources_yield_nulls_and_zero_counts():
    fv = extract_features("MER-1", WINDOW_START, WINDOW_END, {})
    # quantile/ratio features are undefined without data -> None (missing)
    assert fv.values["webhook_delivery_latency_p95_s"] is None
    assert fv.values["handler_error_rate"] is None
    assert fv.values["capture_to_order_update_s"] is None
    # count features observe absence -> 0.0
    assert fv.values["webhook_retry_count"] == 0.0
    assert fv.values["refund_velocity_per_hour"] == 0.0
    assert set(fv.provenance["missing_features"]) == {
        "webhook_delivery_latency_p95_s",
        "handler_error_rate",
        "capture_to_order_update_s",
    }


def test_failed_webhook_counts_as_retry_and_error():
    data = {"webhook_events": [_webhook(status="FAILED", processed=None)]}
    fv = extract_features("MER-1", WINDOW_START, WINDOW_END, data)
    assert fv.values["webhook_retry_count"] == 1.0
    assert fv.values["handler_error_rate"] == 1.0
    assert fv.values["webhook_delivery_latency_p95_s"] is None  # no processed_at


def test_records_outside_window_are_excluded():
    data = {"webhook_events": [_webhook(received="2026-11-05T10:00:00Z",
                                        processed="2026-11-05T10:00:09Z")]}
    fv = extract_features("MER-1", WINDOW_START, WINDOW_END, data)
    assert fv.values["webhook_delivery_latency_p95_s"] is None
    assert fv.provenance["sources_in_window"]["webhook_events"] == 0


def test_cross_merchant_records_strict_mode_raises():
    data = {"webhook_events": [_webhook(merchant="MER-EVIL")]}
    with pytest.raises(TenantMismatchError):
        extract_features("MER-1", WINDOW_START, WINDOW_END, data)


def test_cross_merchant_records_lenient_mode_drops():
    data = {
        "webhook_events": [_webhook(merchant="MER-EVIL"), _webhook(merchant="MER-1")],
    }
    fv = extract_features("MER-1", WINDOW_START, WINDOW_END, data, strict_tenant=False)
    assert fv.provenance["dropped_other_tenant_records"] == 1
    assert fv.provenance["sources_in_window"]["webhook_events"] == 1


def test_invalid_window_rejected():
    with pytest.raises(ValueError):
        extract_features("MER-1", "bogus", WINDOW_END, {})
    with pytest.raises(ValueError):
        extract_features("MER-1", WINDOW_END, WINDOW_START, {})  # end <= start
