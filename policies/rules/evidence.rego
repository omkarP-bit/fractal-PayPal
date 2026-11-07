package fractal.rules.evidence

import rego.v1
import data.fractal.lib

findings(ctx) := findings if {
    max_age := data.fractal.config.max_evidence_age_seconds;

    results := [
        f | f := lib.finding("EVD-001", "WAIT", "EVIDENCE_STALE", "Provider state is older than allowed"); max_age > lib.evidence_age_seconds(ctx)
    ] + [
        f | f := lib.finding("EVD-002", "WAIT", "EVIDENCE_MISSING", "Provider facts are missing"); not ctx.facts.provider.fetched_at
    ] + [
        f | f := lib.gate("evidence_fresh", "EVD-101", "Provider state is fresh"); lib.evidence_age_seconds(ctx) <= max_age
    ]
}
