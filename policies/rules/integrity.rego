package fractal.rules.integrity

import rego.v1
import data.fractal.lib

findings(ctx) := results if {
    results := [
        f | f := lib.finding("INT-001", "BLOCK", "INPUT_SCHEMA_UNSUPPORTED", "Unsupported input schema version"); not ctx.schema_version in data.fractal.config.supported_input_versions
    ] + [
        f | f := lib.finding("INT-004", "BLOCK", "POLICY_VERSION_UNKNOWN", "Policy revision is not set"); object.get(data.fractal.meta, ["revision"], "UNKNOWN") == "UNKNOWN"
    ] + [
        f | f := lib.finding("INT-003", "BLOCK", "INPUT_INCOMPLETE", "Missing input section: request"); not ctx.request
    ] + [
        f | f := lib.finding("INT-003", "BLOCK", "INPUT_INCOMPLETE", "Missing input section: case"); not ctx.case
    ] + [
        f | f := lib.finding("INT-003", "BLOCK", "INPUT_INCOMPLETE", "Missing input section: actor"); not ctx.actor
    ] + [
        f | f := lib.finding("INT-003", "BLOCK", "INPUT_INCOMPLETE", "Missing input section: merchant"); not ctx.merchant
    ] + [
        f | f := lib.finding("INT-003", "BLOCK", "INPUT_INCOMPLETE", "Missing input section: facts"); not ctx.facts
    ] + [
        f | f := lib.finding("INT-003", "BLOCK", "INPUT_INCOMPLETE", "Missing input section: execution"); not ctx.execution
    ] + [
        f | f := lib.finding("INT-003", "BLOCK", "INPUT_INCOMPLETE", "Missing input section: risk"); not ctx.risk
    ]
}
