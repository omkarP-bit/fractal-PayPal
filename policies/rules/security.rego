package fractal.rules.security

import rego.v1
import data.fractal.lib

# Note: approval_satisfied gates live solely in rules/approvals.rego (APR-101/APR-102).
# This module owns webhook, execution and actor authorization gates.

findings(ctx) := results if {
    results := {f |
        f := lib.finding("SEC-001", "BLOCK", "WEBHOOK_SIGNATURE_INVALID", "Webhook signature not verified")
        ctx.facts.webhook.signature_verified == false
    } |
        {f |
            f := lib.finding("SEC-002", "BLOCK", "ACTION_ALREADY_EXECUTED", "Action already executed for this key")
            ctx.execution.already_executed == true
        } |
        {f |
            required_permission := object.get(object.get(data.fractal.actions, [ctx.request.action.type], {}), "required_permission", "")
            f := lib.finding("SEC-003", "BLOCK", "PERMISSION_DENIED", "Actor lacks the required permission")
            not required_permission in ctx.actor.permissions
        } |
        {f |
            f := lib.finding("SEC-004", "BLOCK", "TENANT_MISMATCH", "Actor does not belong to this merchant")
            ctx.actor.merchant_id != ctx.merchant.id
        } |
        {f |
            f := lib.gate("webhook_verified", "SEC-101", "Webhook signature verified")
            ctx.facts.webhook.signature_verified == true
        } |
        {f |
            f := lib.gate("not_executed", "SEC-102", "Action not yet executed")
            ctx.execution.already_executed == false
        } |
        {f |
            required_permission := object.get(object.get(data.fractal.actions, [ctx.request.action.type], {}), "required_permission", "")
            f := lib.gate("actor_permitted", "SEC-103", "Actor permission and tenant verified")
            required_permission in ctx.actor.permissions
        }
}
