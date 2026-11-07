package fractal.rules.approvals

import rego.v1
import data.fractal.lib

findings(ctx) := findings if {
    action_cfg := object.get(data.fractal.actions, [ctx.request.action.type], {});
    merchant_cfg := object.get(data.fractal.merchants, ctx.merchant.id, {});
    exceeds := merchant_cfg.approval_threshold_minor < ctx.request.action.params.amount_minor;
    needs_approval := action_cfg.requires_approval == true;
    needs_approval := exceeds == true;

    results := [
        f | f := lib.finding("APR-001", "BLOCK", "REQUIRE_APPROVAL",
            "A valid approval is required",
            object.get(action_cfg, "approver_role", "MERCHANT_ADMIN"));
        needs_approval; not valid_approval
    ] + [
        f | f := lib.gate("approval_satisfied", "APR-101", "Valid approval present"); needs_approval; valid_approval
    ] + [
        f | f := lib.gate("approval_satisfied", "APR-102", "No approval required"); not needs_approval
    ]
}
