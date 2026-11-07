package fractal.guard_test

import rego.v1
import data.fractal.guard
import data.fractal.fixtures

test_reconcile_allowed if {
    d := guard.decision with input as fixtures.reconcile_ok
    d.outcome == "ALLOW"
}

test_invalid_signature_blocks if {
    inp := json.patch(fixtures.reconcile_ok, [{"op": "replace", "path": "/facts/webhook/signature_verified", "value": false}])
    d := guard.decision with input as inp
    d.outcome == "BLOCK"
}

test_amount_mismatch_blocks if {
    inp := json.patch(fixtures.reconcile_ok, [{"op": "replace", "path": "/facts/order/amount_minor", "value": 5000}])
    d := guard.decision with input as inp
    d.outcome == "BLOCK"
}

test_stale_evidence_waits if {
    inp := json.patch(fixtures.reconcile_ok, [{"op": "replace", "path": "/facts/provider/fetched_at", "value": "2026-11-07T09:00:00Z"}])
    d := guard.decision with input as inp
    d.outcome == "WAIT"
}

test_refund_requires_approval if {
    d := guard.decision with input as fixtures.refund_no_approval
    d.outcome == "REQUIRE_APPROVAL"
}

test_unknown_action_blocks if {
    inp := json.patch(fixtures.reconcile_ok, [{"op": "replace", "path": "/request/action/type", "value": "DELETE_EVERYTHING"}])
    d := guard.decision with input as inp
    d.outcome == "BLOCK"
}

test_agent_advice_alone_cannot_allow if {
    d := guard.decision with input as fixtures.agent_only
    d.outcome != "ALLOW"
}
