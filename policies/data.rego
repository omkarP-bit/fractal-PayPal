package policy_data

config := {
    "supported_input_versions": ["1.0"],
    "max_evidence_age_seconds": 120,
    "anomaly_approval_threshold": 0.8
}

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
    }
}

meta := {
    "revision": "2026-11-07.1"
}

packs := {
    "us_reg_e": {
        "enabled": false,
        "legal_review": "PENDING"
    }
}
