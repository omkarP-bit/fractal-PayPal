"""Pydantic schemas for the LangGraph agent investigation contract."""

from pydantic import BaseModel, Field, ConfigDict
from typing import List, Optional


class InvestigateRequest(BaseModel):
    case_id: str
    run_number: int
    case_type: str
    customer_description: str


class InvestigationSubmission(BaseModel):
    model_config = ConfigDict(str_strip_whitespace=True, protected_namespaces=())

    run_number: int = Field(..., ge=1)
    category: str = Field(..., min_length=1, max_length=100)
    root_cause_hypothesis: str = Field(..., min_length=1, max_length=2000)
    evidence_ids: List[str] = Field(default_factory=list)
    missing_evidence: List[str] = Field(default_factory=list)
    recommendation: str = Field(..., min_length=1, max_length=100)
    suggested_severity: str = Field(default="MEDIUM", pattern="^(LOW|MEDIUM|HIGH|CRITICAL)$")
    customer_explanation: Optional[str] = Field(default=None, max_length=2000)
    merchant_fix_suggestion: Optional[str] = Field(default=None, max_length=2000)
    agent_version: str = Field(default="investigator/1.0", max_length=100)
    model_id: str = Field(default="rules-v1", max_length=100)


class InvestigationResponse(BaseModel):
    status: str
    run_number: int
    case_id: str
    validation_status: str
    submission: InvestigationSubmission
