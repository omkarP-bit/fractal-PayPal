package fractal.rules.paypal

import rego.v1
import data.fractal.lib

findings(ctx) := results if {
    results := {f |
        f := lib.finding("PP-001", "WAIT", "PROVIDER_NOT_COMPLETED", "Provider has not reported COMPLETED")
        ctx.facts.provider.name == "PAYPAL"
        ctx.facts.provider.status != "COMPLETED"
    } |
        {f |
            f := lib.finding("PP-002", "BLOCK", "AMOUNT_MISMATCH", "Provider and order amounts differ")
            ctx.facts.provider.name == "PAYPAL"
            ctx.facts.provider.amount_minor != ctx.facts.order.amount_minor
        } |
        {f |
            f := lib.finding("PP-003", "BLOCK", "CURRENCY_MISMATCH", "Provider and order currencies differ")
            ctx.facts.provider.name == "PAYPAL"
            ctx.facts.provider.currency != ctx.facts.order.currency
        } |
        {f |
            f := lib.gate("provider_completed", "PP-101", "Provider reports COMPLETED")
            ctx.facts.provider.name == "PAYPAL"
            ctx.facts.provider.status == "COMPLETED"
        } |
        {f |
            f := lib.gate("amounts_match", "PP-102", "Amounts match")
            ctx.facts.provider.name == "PAYPAL"
            ctx.facts.provider.amount_minor == ctx.facts.order.amount_minor
        }
}
