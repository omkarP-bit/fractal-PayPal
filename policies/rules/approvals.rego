package fractal.rules.approvals

import rego.v1
import data.fractal.lib

# valid_approval reports whether ctx contains an APPROVED approval bound to
# this exact action type and params hash (Phase 8 binding semantics).
valid_approval(ctx) := true if {
    some a in object.get(ctx, "approvals", [])
    a.action_type == ctx.request.action.type
    a.params_sha256 == ctx.request.action.params_sha256
    a.decision == "APPROVED"
}

# needs_approval is true when the action requires approval (action config) or
# the amount exceeds the merchant approval threshold (OR of the two).
needs_approval(ctx) := true if {
    action_cfg := object.get(data.fractal.actions, [ctx.request.action.type], {})
    object.get(action_cfg, "requires_approval", false) == true
}

needs_approval(ctx) := true if {
    merchant_cfg := object.get(data.fractal.merchants, ctx.merchant.id, {})
    object.get(merchant_cfg, "approval_threshold_minor", 0) <
        object.get(ctx.request.action, ["params", "amount_minor"], 0)
}

findings(ctx) := results if {
    results := {f |
        action_cfg := object.get(data.fractal.actions, [ctx.request.action.type], {})
        f := {
            "rule_id": "APR-001",
            "outcome": "REQUIRE_APPROVAL",
            "code": "APPROVAL_REQUIRED",
            "message": "A valid approval is required",
            "approver_role": object.get(action_cfg, "approver_role", "MERCHANT_ADMIN"),
        }
        needs_approval(ctx)
        not valid_approval(ctx)
    } |
        {f |
            f := lib.gate("approval_satisfied", "APR-101", "Valid approval present")
            needs_approval(ctx)
            valid_approval(ctx)
        } |
        {f |
            f := lib.gate("approval_satisfied", "APR-102", "No approval required")
            not needs_approval(ctx)
        }
}
