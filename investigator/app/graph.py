"""LangGraph agent graph — single agent with read-only tools only.

The node is deterministic and evidence-driven. An optional LLM pass (see
llm.py) can replace its output; Go re-verifies every reference either way.
"""

from typing import TypedDict, List, Optional

from langgraph.graph import StateGraph, END


class AgentState(TypedDict, total=False):
    case_id: str
    run_number: int
    case_type: str
    customer_description: str
    evidence: List[dict]
    webhook_events: List[dict]
    logs: List[dict]
    category: str
    root_cause_hypothesis: str
    evidence_ids: List[str]
    missing_evidence: List[str]
    recommendation: str
    suggested_severity: str
    customer_explanation: str
    merchant_fix_suggestion: str
    agent_version: str
    model_id: str


def _field(payload: Optional[dict], *names: str, default=None):
    if not isinstance(payload, dict):
        return default
    for name in names:
        if name in payload and payload[name] is not None:
            return payload[name]
    return default


def _first_of_type(evidence: List[dict], kind: str) -> Optional[dict]:
    for row in evidence:
        if row.get("evidence_type") == kind:
            return row
    return None


def derive_findings(state: AgentState) -> AgentState:
    evidence = state.get("evidence") or []
    events = state.get("webhook_events") or []

    provider = _first_of_type(evidence, "PROVIDER_TRANSACTION")
    order = _first_of_type(evidence, "ORDER_SNAPSHOT")
    provider_payload = (provider or {}).get("payload") or {}
    order_payload = (order or {}).get("payload") or {}

    provider_status = str(_field(provider_payload, "Status", "status", default="")).upper()
    order_status = str(_field(order_payload, "Status", "status", default="")).upper()
    provider_amount = _field(provider_payload, "AmountMinor", "amount_minor")
    order_amount = _field(order_payload, "AmountMinor", "amount_minor")
    provider_currency = _field(provider_payload, "Currency", "currency", default="")
    order_currency = _field(order_payload, "Currency", "currency", default="")

    missing = []
    if provider is None:
        missing.append("PROVIDER_TRANSACTION")
    if order is None:
        missing.append("ORDER_SNAPSHOT")
    if not events:
        missing.append("WEBHOOK_EVENT")

    evidence_ids = [row.get("id") for row in evidence if row.get("id")]
    webhook_evidence_ids = [
        row.get("id") for row in evidence
        if row.get("evidence_type") == "WEBHOOK_EVENT" and row.get("id")
    ]
    evidence_ids.extend(webhook_evidence_ids)
    evidence_ids = list(dict.fromkeys(evidence_ids))

    amount_mismatch = (
        provider_amount is not None
        and order_amount is not None
        and str(provider_amount) != str(order_amount)
    )
    currency_mismatch = bool(provider_currency and order_currency and provider_currency != order_currency)

    if provider is None or order is None:
        category = "INSUFFICIENT_EVIDENCE"
        severity = "MEDIUM"
        recommendation = "NO_ACTION"
        hypothesis = (
            "Some required evidence was not available, so the provider and merchant "
            "states cannot be compared. Missing: " + ", ".join(missing) + "."
        )
        customer_explanation = "We need a little more payment evidence before we can resolve this case."
        merchant_fix = "Re-run evidence collection and confirm the payment transaction is linked to the case."
    elif amount_mismatch or currency_mismatch:
        category = "AMOUNT_MISMATCH"
        severity = "HIGH"
        recommendation = "NO_ACTION"
        hypothesis = (
            f"PayPal reports {provider_amount} {provider_currency} while the merchant order "
            f"records {order_amount} {order_currency}. The amounts or currencies disagree."
        )
        customer_explanation = "The payment amount and the order amount do not match, so we are reviewing it."
        merchant_fix = "Compare the PayPal capture amount with the order amount before fulfilling."
    elif provider_status == "COMPLETED" and order_status in ("PAYMENT_PENDING", "PENDING", "CREATED"):
        category = "PROVIDER_MERCHANT_STATE_MISMATCH"
        severity = "MEDIUM"
        recommendation = "RECONCILE_MERCHANT_ORDER"
        delivery = (
            "Verified webhook events exist for this order, so the merchant handler probably failed after delivery."
            if events
            else "No verified webhook event was stored for this order, so the payment notification never reached the merchant."
        )
        hypothesis = (
            f"PayPal reports the payment as COMPLETED, but the merchant order is still {order_status}. "
            + delivery
        )
        customer_explanation = (
            "PayPal confirms your payment. The store has not updated the order yet, so we are reconciling it."
        )
        merchant_fix = (
            "Mark the order paid from the verified PayPal capture, then check the webhook handler and retry delivery."
        )
    elif provider_status == "COMPLETED" and order_status == "PAID":
        category = "ORDER_ALREADY_RECONCILED"
        severity = "LOW"
        recommendation = "NO_ACTION"
        hypothesis = "PayPal and the merchant both report the order as paid. No reconciliation is needed."
        customer_explanation = "Your payment and the store order already agree. Nothing further is needed."
        merchant_fix = "No action needed."
    elif provider_status in ("REFUNDED", "PARTIALLY_REFUNDED"):
        category = "PAYMENT_REFUNDED"
        severity = "LOW"
        recommendation = "NO_ACTION"
        hypothesis = "PayPal reports the payment as refunded."
        customer_explanation = "This payment was refunded."
        merchant_fix = "Confirm the refund is reflected in the order system."
    else:
        category = "PAYMENT_NOT_COMPLETED"
        severity = "HIGH"
        recommendation = "NO_ACTION"
        hypothesis = (
            f"PayPal reports the payment as {provider_status or 'UNKNOWN'}, so it cannot be reconciled as captured."
        )
        customer_explanation = "PayPal has not completed this payment, so the order was not marked paid."
        merchant_fix = "Do not fulfil the order until PayPal reports a completed capture."

    return {
        "category": category,
        "root_cause_hypothesis": hypothesis,
        "evidence_ids": evidence_ids,
        "missing_evidence": missing,
        "recommendation": recommendation,
        "suggested_severity": severity,
        "customer_explanation": customer_explanation,
        "merchant_fix_suggestion": merchant_fix,
    }


def investigate_node(state: AgentState) -> AgentState:
    """Single LangGraph node: correlates evidence and produces findings."""
    return derive_findings(state)


def build_graph():
    workflow = StateGraph(AgentState)
    workflow.add_node("investigate", investigate_node)
    workflow.set_entry_point("investigate")
    workflow.add_edge("investigate", END)
    return workflow.compile()
