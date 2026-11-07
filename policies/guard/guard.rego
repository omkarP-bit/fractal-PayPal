package fractal.guard

import rego.v1
import data.fractal.lib

decision := {
    "outcome": "ALLOW",
    "action": "RECONCILE_MERCHANT_ORDER",
    "findings": [],
    "missing_gates": [],
    "required_approvals": [],
    "policy": {
        "revision": "2026-11-07.1",
        "schema_version": "1.0"
    },
}
