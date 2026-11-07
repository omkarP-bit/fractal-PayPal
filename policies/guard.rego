package fractal.guard

import rego.v1
import data.fractal.lib

input_ctx := {
    "schema_version": input.schema_version,
    "request": input.request,
    "case": input.case,
    "actor": inp.actor,
    "merchant": inp.merchant,
    "facts": inp.facts,
    "execution": inp.execution,
    "risk": inp.risk,
    "approvals": inp.approvals,
    "applicability": inp.applicability,
    "advisory": inp.advisory,
}

module_findings contains f if {
    f := data.fractal.rules.integrity.findings(input_ctx)[_]
}
module_findings contains f if {
    f := data.fractal.rules.security.findings(input_ctx)[_]
}
module_findings contains f if {
    f := data.fractal.rules.evidence.findings(input_ctx)[_]
}
module_findings contains f if {
    f := data.fractal.rules.paypal.findings(input_ctx)[_]
}
module_findings contains f if {
    f := data.fractal.rules.approvals.findings(input_ctx)[_]
}
module_findings contains f if {
    f := data.fractal.rules.risk.findings(input_ctx)[_]
}

gates_passed contains f.gate if {
    some f in module_findings
    f.outcome == "ALLOW"
    f.gate
}

required_gates["webhook_verified"] := true
required_gates["not_executed"] := true
required_gates["actor_permitted"] := input_ctx.request.action.type == "RECONCILE_MERCHANT_ORDER"
required_gates["evidence_fresh"] := true
required_gates["provider_completed"] := true
required_gates["amounts_match"] := true
required_gates["approval_satisfied"] := input_ctx.request.action.type == "RECONCILE_MERCHANT_ORDER"
required_gates["risk_acceptable"] := true

missing_gates := {g | some g in required_gates; not g in gates_passed}

gate_findings contains lib.finding("GRD-001", "BLOCK", "REQUIRED_GATE_MISSING",
    sprintf("Required gate not satisfied: %s", [g])) if {
    some g in missing_gates
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
