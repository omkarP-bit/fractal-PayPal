package fractal.anomaly_test

import rego.v1
import data.fractal.guard
import data.fractal.fixtures

# Phase 11 invariants for the anomaly signal (tighten-only).
#
# Invariant 1: disabled (shipped default) == Phase 10, even for a high score.
# Invariant 2: null score == Phase 10, even when the model is enabled.
# Invariant 3: a high score may escalate ALLOW to REQUIRE_APPROVAL.
# Invariant 4: BLOCK never becomes ALLOW under any anomaly score.
# Invariant 5: the anomaly signal never independently authorizes an action.
# Invariant 6: existing WAIT/evidence/approval gates stay intact.

# with_score patches the anomaly score onto a fixture.
with_score(fixture, score) := json.patch(fixture, [
    {"op": "replace", "path": "/risk/anomaly_score", "value": score},
])

has_rsk002(d) := true if {
    some f in d.findings
    f.rule_id == "RSK-002"
}

# --- Invariant 1: disabled == Phase 10 -------------------------------------

test_disabled_high_score_equals_phase10 if {
    d := guard.decision with input as with_score(fixtures.reconcile_ok, 0.97)
    d.outcome == "ALLOW"
    not has_rsk002(d)
}

test_disabled_decision_identical_to_null_score if {
    base := guard.decision with input as fixtures.reconcile_ok
    with_high := guard.decision with input as with_score(fixtures.reconcile_ok, 0.99)
    base == with_high
}

# --- Invariant 2: null score == Phase 10 (even when enabled) ---------------

test_null_score_equals_phase10_even_when_enabled if {
    d := guard.decision with input as fixtures.reconcile_ok with data.fractal.anomaly_detection_enabled as true
    d.outcome == "ALLOW"
    not has_rsk002(d)
}

test_enabled_null_identical_to_disabled if {
    disabled := guard.decision with input as fixtures.reconcile_ok
    enabled_null := guard.decision with input as fixtures.reconcile_ok with data.fractal.anomaly_detection_enabled as true
    disabled == enabled_null
}

# --- Invariant 3: high score escalates to REQUIRE_APPROVAL ----------------

test_high_score_escalates_to_require_approval if {
    d := guard.decision with input as with_score(fixtures.reconcile_ok, 0.95) with data.fractal.anomaly_detection_enabled as true
    d.outcome == "REQUIRE_APPROVAL"
    has_rsk002(d)
    "MERCHANT_ADMIN" in d.required_approvals
}

test_score_below_threshold_stays_allow if {
    d := guard.decision with input as with_score(fixtures.reconcile_ok, 0.42) with data.fractal.anomaly_detection_enabled as true
    d.outcome == "ALLOW"
    not has_rsk002(d)
}

# --- Invariant 4: BLOCK never becomes ALLOW -------------------------------

test_block_stays_block_invalid_signature_high_score if {
    inp := json.patch(with_score(fixtures.reconcile_ok, 0.99), [
        {"op": "replace", "path": "/facts/webhook/signature_verified", "value": false},
    ])
    d := guard.decision with input as inp with data.fractal.anomaly_detection_enabled as true
    d.outcome == "BLOCK"
}

test_block_stays_block_amount_mismatch_high_score if {
    inp := json.patch(with_score(fixtures.reconcile_ok, 0.99), [
        {"op": "replace", "path": "/facts/order/amount_minor", "value": 5000},
    ])
    d := guard.decision with input as inp with data.fractal.anomaly_detection_enabled as true
    d.outcome == "BLOCK"
}

# --- Invariant 5: anomaly never independently authorizes ------------------

test_anomaly_never_authorizes_refund_without_approval if {
    d := guard.decision with input as with_score(fixtures.refund_no_approval, 0.99) with data.fractal.anomaly_detection_enabled as true
    d.outcome == "REQUIRE_APPROVAL"
    d.outcome != "ALLOW"
}

test_anomaly_never_flips_unknown_action_to_allow if {
    inp := json.patch(with_score(fixtures.reconcile_ok, 0.99), [
        {"op": "replace", "path": "/request/action/type", "value": "DELETE_EVERYTHING"},
    ])
    d := guard.decision with input as inp with data.fractal.anomaly_detection_enabled as true
    d.outcome == "BLOCK"
}

# --- Invariant 6: WAIT / evidence gates intact -----------------------------

test_wait_gate_intact_when_anomaly_high if {
    inp := json.patch(with_score(fixtures.reconcile_ok, 0.95), [
        {"op": "replace", "path": "/facts/provider/fetched_at", "value": "2026-11-07T09:00:00Z"},
    ])
    d := guard.decision with input as inp with data.fractal.anomaly_detection_enabled as true
    d.outcome == "WAIT"
}
