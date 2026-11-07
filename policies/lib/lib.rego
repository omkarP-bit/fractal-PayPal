package fractal.lib

import rego.v1

# finding produces a standard finding object emitted by rule modules.
finding(rule_id, outcome, code, message) := {
    "rule_id": rule_id,
    "outcome": outcome,
    "code": code,
    "message": message,
}

# gate produces a standard gate-finding object (ALLOW outcome with a gate name).
gate(name, rule_id, message) := {
    "rule_id": rule_id,
    "outcome": "ALLOW",
    "gate": name,
    "code": "GATE_PASSED",
    "message": message,
}

# evidence_age_seconds computes the age in seconds between evaluated_at and fetched_at.
# Takes ctx (input context) instead of reading input directly, to avoid recursion
# with `with input as` in OPA tests.
evidence_age_seconds(ctx) := age if {
    now_ns := time.parse_rfc3339_ns(ctx.request.evaluated_at)
    fetched_ns := time.parse_rfc3339_ns(ctx.facts.provider.fetched_at)
    age := (now_ns - fetched_ns) / 1000000000
}
