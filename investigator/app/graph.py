"""LangGraph agent graph — single agent with read-only tools only."""

from langgraph.graph import StateGraph, END
from typing import TypedDict, List, Optional


class AgentState(TypedDict, total=False):
    case_id: str
    run_number: int
    case_type: str
    customer_description: str
    evidence: List[dict]
    webhook_events: List[dict]
    logs: List[dict]
    traces: List[dict]
    error_rate: Optional[dict]
    category: str
    root_cause_hypothesis: str
    evidence_ids: List[str]
    missing_evidence: List[str]
    recommendation: str
    suggested_severity: str
    customer_explanation: str
    merchant_fix_suggestion: str


def investigate_node(state: AgentState) -> AgentState:
    """Single LangGraph node: correlates evidence and produces findings."""
    # This is a stub structure. The actual LLM call is wired in main.py.
    return {
        "category": "PROVIDER_MERCHANT_STATE_MISMATCH",
        "root_cause_hypothesis": "Pending LLM investigation",
        "evidence_ids": [],
        "missing_evidence": [],
        "recommendation": "RECONCILE_MERCHANT_ORDER",
        "suggested_severity": "MEDIUM",
        "customer_explanation": "Investigation in progress.",
        "merchant_fix_suggestion": "Review webhook handler logs.",
    }


def build_graph():
    workflow = StateGraph(AgentState)
    workflow.add_node("investigate", investigate_node)
    workflow.set_entry_point("investigate")
    workflow.add_edge("investigate", END)
    return workflow.compile()

