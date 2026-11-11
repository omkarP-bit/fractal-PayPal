"""Synthetic generator tests: labeling, determinism, overwrite refusal."""
import json
import os

import pytest

from app.anomaly.synthetic import (
    DATA_ORIGIN,
    SCENARIOS,
    generate_dataset,
    generate_window,
    write_dataset,
)
from datetime import datetime, timezone

START = datetime(2026, 11, 1, tzinfo=timezone.utc)


def test_every_window_is_labeled_synthetic():
    dataset = generate_dataset(merchants=2, seed=3)
    assert dataset["data_origin"] == DATA_ORIGIN
    assert dataset["synthetic"] is True
    assert "not real" in dataset["disclaimer"].lower() or "synthetic" in dataset["disclaimer"].lower()
    for window in dataset["windows"]:
        assert window["data_origin"] == DATA_ORIGIN
        assert window["synthetic"] is True
        assert window["scenario"] in SCENARIOS
        for source_rows in window["records"].values():
            for row in source_rows:
                assert row["merchant_id"] == window["merchant_id"]


def test_unknown_scenario_rejected():
    with pytest.raises(ValueError):
        generate_window("MER-1", START, scenario="not-a-scenario")


def test_deterministic_for_same_seed():
    a = generate_window("MER-1", START, scenario="normal", seed=5)
    b = generate_window("MER-1", START, scenario="normal", seed=5)
    assert a == b
    c = generate_window("MER-1", START, scenario="normal", seed=6)
    assert a != c


def test_burst_scenario_increases_webhook_failures():
    normal = generate_window("MER-1", START, scenario="normal", seed=9)
    burst = generate_window("MER-1", START, scenario="webhook_failure_burst", seed=9)
    normal_failed = sum(1 for r in normal["records"]["webhook_events"]
                        if r["processing_status"] == "FAILED")
    burst_failed = sum(1 for r in burst["records"]["webhook_events"]
                       if r["processing_status"] == "FAILED")
    assert burst_failed > normal_failed
    normal_rejects = sum(1 for r in normal["records"]["security_events"]
                         if r["event_type"].startswith("WEBHOOK_"))
    burst_rejects = sum(1 for r in burst["records"]["security_events"]
                        if r["event_type"].startswith("WEBHOOK_"))
    assert burst_rejects >= 8  # the "8x webhook failures" demo moment


def test_refund_storm_increases_refunds():
    normal = generate_window("MER-1", START, scenario="normal", seed=11)
    storm = generate_window("MER-1", START, scenario="refund_storm", seed=11)
    count = lambda ds: sum(1 for r in ds["records"]["action_executions"]  # noqa: E731
                           if r["action_type"] == "REFUND_PAYMENT")
    assert count(storm) > count(normal)
    assert count(storm) >= 10


def test_write_dataset_refuses_overwrite(tmp_path):
    path = os.path.join(tmp_path, "dataset.json")
    dataset = generate_dataset(merchants=1, seed=1)
    assert write_dataset(path, dataset) == path
    assert os.path.exists(path)
    # second write to the same path must fail — prior artifact preserved
    with pytest.raises(FileExistsError):
        write_dataset(path, dataset)
    # original content intact
    with open(path, encoding="utf-8") as fh:
        loaded = json.load(fh)
    assert loaded["seed"] == dataset["seed"]


def test_write_dataset_never_touches_seed_files(tmp_path):
    # Guard: generator output path is caller-controlled but the refusal rule
    # applies everywhere; simulate an existing seed file and assert refusal.
    seed_file = os.path.join(tmp_path, "seed_demo_data.sql")
    with open(seed_file, "w", encoding="utf-8") as fh:
        fh.write("-- existing phase 10 seed, do not touch\n")
    with pytest.raises(FileExistsError):
        write_dataset(seed_file, generate_dataset(merchants=1, seed=1))
    with open(seed_file, encoding="utf-8") as fh:
        assert "phase 10 seed" in fh.read()
