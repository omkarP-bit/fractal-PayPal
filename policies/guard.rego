package fractal.guard

import rego.v1
import data.fractal.lib

# input_ctx normalizes the raw input. Missing sections become false (not
# undefined) so INT-003 can report them instead of the whole context collapsing.
input_ctx := {
    "schema_version": object.get(input, "schema_version", "UNKNOWN"),
    "request": object.get(input, "request", false),
    "case": object.get(input, "case", false),
    "actor": object.get(input, "actor", false),
    "merchant": object.get(input, "merchant", false),
    "facts": object.get(input, "facts", false),
    "execution": object.get(input, "execution", false),
    "risk": object.get(input, "risk", false),
    "approvals": object.get(input, "approvals", []),
    "applicability": object.get(input, "applicability", []),
    "advisory": object.get(input, "advisory", false),
}

module_findings contains f if {
    some f in data.fractal.rules.integrity.findings(input_ctx)
}

module_findings contains f if {
    some f in data.fractal.rules.security.findings(input_ctx)
}

module_findings contains f if {
    some f in data.fractal.rules.evidence.findings(input_ctx)
}

module_findings contains f if {
    some f in data.fractal.rules.paypal.findings(input_ctx)
}

module_findings contains f if {
    some f in data.fractal.rules.approvals.findings(input_ctx)
}

module_findings contains f if {
    some f in data.fractal.rules.risk.findings(input_ctx)
}

gates_passed contains f.gate if {
    some f in module_findings
    f.outcome == "ALLOW"
    f.gate
}

# required_gates maps gate name -> required-for-this-action. A false value
# means the gate is not required for this action type; an undefined value
# expression (e.g. no request section) omits the key entirely.
required_gates["webhook_verified"] := true
required_gates["not_executed"] := true
required_gates["actor_permitted"] := input_ctx.request.action.type == "RECONCILE_MERCHANT_ORDER"
required_gates["evidence_fresh"] := true
required_gates["provider_completed"] := true
required_gates["amounts_match"] := true
required_gates["approval_satisfied"] := input_ctx.request.action.type == "RECONCILE_MERCHANT_ORDER"
required_gates["risk_acceptable"] := true

# Missing = required (true) and not passed. Iterates keys, not values.
missing_gates := {g | some g, req in required_gates; req == true; not g in gates_passed}

# True when a rule module already reported BLOCK/WAIT/REQUIRE_APPROVAL for
# this input. In that case the module's own outcome governs and GRD-001
# must not escalate it (e.g. stale evidence is WAIT via EVD-001, not BLOCK).
module_has_severity if {
    some f in module_findings
    f.outcome != "ALLOW"
}

gate_findings contains lib.finding("GRD-001", "BLOCK", "REQUIRED_GATE_MISSING",
    sprintf("Required gate not satisfied: %s", [g])) if {
    some g in missing_gates
    not module_has_severity
}

gate_findings contains lib.finding("GRD-002", "BLOCK", "NO_GATES_DEFINED",
    "Action defines no required gates") if {
    count(required_gates) == 0
}

all_findings := module_findings | gate_findings

has(o) if {
    some f in all_findings
    f.outcome == o
}

default outcome := "BLOCK"

outcome := "BLOCK" if has("BLOCK")

outcome := "WAIT" if {
    not has("BLOCK")
    has("WAIT")
}

outcome := "REQUIRE_APPROVAL" if {
    not has("BLOCK")
    not has("WAIT")
    has("REQUIRE_APPROVAL")
}

outcome := "ALLOW" if {
    not has("BLOCK")
    not has("WAIT")
    not has("REQUIRE_APPROVAL")
    has("ALLOW")
}

decision := {
    "outcome": outcome,
    "action": object.get(input_ctx, ["request", "action", "type"], null),
    "findings": [f | some f in all_findings],
    "missing_gates": [g | some g in missing_gates],
    "required_approvals": [f.approver_role | some f in all_findings; f.code == "APPROVAL_REQUIRED"],
    "policy": {
        "revision": "2026-11-07.1",
        "schema_version": object.get(input_ctx, "schema_version", "UNKNOWN"),
    },
}
