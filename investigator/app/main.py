"""FastAPI entry point for the FRACTAL AI investigation service.

Single LangGraph agent with read-only tools only. Output is advisory —
Go re-verifies facts and builds the OPA input.
"""

from fastapi import FastAPI, HTTPException
from pydantic import ValidationError

from .schemas import InvestigateRequest, InvestigationSubmission, InvestigationResponse
from .graph import build_graph
from .tools import AgentTools

app = FastAPI(title="FRACTAL Investigator", version="1.0.0")

agent_graph = build_graph()
tools = AgentTools()


@app.get("/healthz")
async def healthz():
    return {"status": "ok"}


@app.post("/v1/investigate")
async def investigate(req: InvestigateRequest):
    """Receive an investigation request from Go, run the agent, return findings."""
    try:
        # Run the LangGraph agent (read-only tools only)
        result = agent_graph.invoke({
            "case_id": req.case_id,
            "run_number": req.run_number,
            "case_type": req.case_type,
            "customer_description": req.customer_description,
        })

        submission = InvestigationSubmission(
            run_number=req.run_number,
            category=result.get("category", "OTHER"),
            root_cause_hypothesis=result.get("root_cause_hypothesis", ""),
            evidence_ids=result.get("evidence_ids", []),
            missing_evidence=result.get("missing_evidence", []),
            recommendation=result.get("recommendation", "RECONCILE_MERCHANT_ORDER"),
            suggested_severity=result.get("suggested_severity", "MEDIUM"),
            customer_explanation=result.get("customer_explanation"),
            merchant_fix_suggestion=result.get("merchant_fix_suggestion"),
        )

        return InvestigationResponse(
            status="submitted",
            run_number=submission.run_number,
            case_id=req.case_id,
            validation_status="SCHEMA_VALID",
        )
    except ValidationError as e:
        raise HTTPException(status_code=422, detail=str(e))
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

