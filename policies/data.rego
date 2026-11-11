package fractal

import rego.v1

config := {
    "supported_input_versions": ["1.0"],
    "max_evidence_age_seconds": 120,
    "anomaly_approval_threshold": 0.8,
}

# Phase 11: the anomaly signal is DISABLED by default. While this is false the
# system behaves exactly as Phase 10 (tighten-only invariant 1). Tests enable it
# with `with data.fractal.anomaly_detection_enabled as true`.
anomaly_detection_enabled := false

actions := {
    "RECONCILE_MERCHANT_ORDER": {
        "required_permission": "reconcile",
        "requires_approval": false,
        "required_gates": ["webhook_verified", "not_executed", "actor_permitted", "evidence_fresh",
                           "provider_completed", "amounts_match", "approval_satisfied", "risk_acceptable"]
    },
    "REFUND_PAYMENT": {
        "required_permission": "refund",
        "requires_approval": true,
        "approver_role": "MERCHANT_ADMIN",
        "required_gates": ["not_executed", "actor_permitted", "evidence_fresh",
                           "provider_completed", "approval_satisfied", "risk_acceptable"]
    }
}

merchants := {
    "MER-501": {
        "approval_threshold_minor": 20000
    },
    "00000000-0000-0000-0000-0000000000c1": {
        "approval_threshold_minor": 20000
    }
}

meta := {
    "revision": "2026-11-07.1"
}

packs := {
    "us_reg_e": {
        "enabled": false,
        "legal_basis": "legitimate_interest",
        "notes": "Example regulation pack; disabled by default."
    },
    "paypal_integration": {
        "enabled": true,
        "legal_basis": "contract",
        "notes": "PayPal Sandbox integration pack."
    }
}
