"""FastAPI entry point for the FRACTAL AI investigation service.

Single LangGraph agent with read-only tools only. Output is advisory —
Go re-verifies facts and builds the OPA input.
"""

import os
from typing import Any, Optional

from fastapi import FastAPI, HTTPException, Request
from pydantic import ValidationError

from .graph import build_graph
from .llm import investigate_with_llm
from .schemas import InvestigateRequest, InvestigationResponse, InvestigationSubmission
from .tools import AgentTools

app = FastAPI(title="FRACTAL Investigator", version="1.0.0")

agent_graph = build_graph()
tools = AgentTools()
INTERNAL_TOKEN = os.getenv("INVESTIGATOR_TOKEN", "")

SEVERITIES = {"LOW", "MEDIUM", "HIGH", "CRITICAL"}
RECOMMENDATIONS = {"RECONCILE_MERCHANT_ORDER", "REFUND_PAYMENT", "NO_ACTION"}


def _authorized(request: Request) -> bool:
    if not INTERNAL_TOKEN:
        return True
    return request.headers.get("X-Fractal-Internal-Token", "") == INTERNAL_TOKEN


def _clean(value: Any, limit: int) -> str:
    if not isinstance(value, str):
        return ""
    return value.strip()[:limit]


def _normalize(data: dict, run_number: int) -> dict:
    severity = str(data.get("suggested_severity") or "MEDIUM").strip().upper()
    if severity not in SEVERITIES:
        severity = "MEDIUM"
    recommendation = str(data.get("recommendation") or "NO_ACTION").strip().upper()
    if recommendation not in RECOMMENDATIONS:
        recommendation = "NO_ACTION"
    evidence_ids = [str(item) for item in (data.get("evidence_ids") or []) if item]
    missing = [str(item) for item in (data.get("missing_evidence") or []) if item]

    return {
        "run_number": run_number,
        "category": _clean(data.get("category"), 100) or "OTHER",
        "root_cause_hypothesis": _clean(data.get("root_cause_hypothesis"), 2000)
        or "No root cause hypothesis was produced.",
        "evidence_ids": evidence_ids,
        "missing_evidence": missing,
        "recommendation": recommendation,
        "suggested_severity": severity,
        "customer_explanation": _clean(data.get("customer_explanation"), 2000) or None,
        "merchant_fix_suggestion": _clean(data.get("merchant_fix_suggestion"), 2000) or None,
        "agent_version": _clean(data.get("agent_version"), 100) or "investigator/1.0",
        "model_id": _clean(data.get("model_id"), 100) or "rules-v1",
    }


async def _collect(case_id: str):
    evidence, events, logs = [], [], []
    try:
        evidence = await tools.case_evidence(case_id)
    except Exception:
        evidence = []
    try:
        events = await tools.webhook_events(case_id)
    except Exception:
        events = []
    try:
        logs = await tools.search_logs(case_id)
    except Exception:
        logs = []
    return evidence, events, logs


@app.get("/healthz")
async def healthz():
    return {"status": "ok"}


@app.post("/v1/investigate")
async def investigate(req: InvestigateRequest, request: Request):
    """Receive an investigation request from Go, run the agent, return findings."""
    if not _authorized(request):
        raise HTTPException(status_code=401, detail="unauthorized")

    evidence, events, logs = await _collect(req.case_id)

    submission_data: Optional[dict] = None
    try:
        candidate = await investigate_with_llm(
            evidence, events, logs, req.case_type, req.customer_description
        )
    except Exception:
        candidate = None
    if candidate is not None:
        normalized = _normalize(candidate, req.run_number)
        try:
            InvestigationSubmission(**normalized)
            submission_data = normalized
        except ValidationError:
            submission_data = None

    if submission_data is None:
        result = agent_graph.invoke({
            "case_id": req.case_id,
            "run_number": req.run_number,
            "case_type": req.case_type,
            "customer_description": req.customer_description,
            "evidence": evidence,
            "webhook_events": events,
            "logs": logs,
        })
        submission_data = _normalize(dict(result), req.run_number)
        submission_data["agent_version"] = "investigator-rules/1.0"
        submission_data["model_id"] = "rules-v1"

    submission = InvestigationSubmission(**submission_data)
    return InvestigationResponse(
        status="submitted",
        run_number=submission.run_number,
        case_id=req.case_id,
        validation_status="SCHEMA_VALID",
        submission=submission,
    )
