package fractal.rules.risk

import rego.v1
import data.fractal.lib

findings(ctx) := results if {
    results := {f |
        f := lib.finding("RSK-001", "BLOCK", "SEVERITY_CRITICAL", "Critical operational risk")
        ctx.risk.severity == "CRITICAL"
    } |
        # RSK-002 (Phase 11): tighten-only anomaly escalation. Fires only when
        # the anomaly model is explicitly enabled AND a numeric score meets the
        # threshold. Disabled (default) or null score => no finding => identical
        # to Phase 10.
        {f |
            f := {
                "rule_id": "RSK-002",
                "outcome": "REQUIRE_APPROVAL",
                "code": "APPROVAL_REQUIRED",
                "message": "Anomaly score above threshold",
                "approver_role": "MERCHANT_ADMIN",
            }
            data.fractal.anomaly_detection_enabled == true
            is_number(ctx.risk.anomaly_score)
            ctx.risk.anomaly_score >= data.fractal.config.anomaly_approval_threshold
        } |
        {f |
            f := lib.gate("risk_acceptable", "RSK-101", "Risk severity acceptable")
            ctx.risk.severity in {"LOW", "MEDIUM", "HIGH"}
        }
}
