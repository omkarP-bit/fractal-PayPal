"""Read-only tools for the LangGraph agent. No write access, no PayPal credentials."""

import os
from typing import List, Optional, Dict, Any

import httpx


FRACTAL_API_BASE: str = os.getenv("FRACTAL_API_BASE", "http://localhost:8080")
INTERNAL_TOKEN: str = os.getenv("INVESTIGATOR_TOKEN", "")


class AgentTools:
    """Read-only tools the agent can call. All return data only."""

    def __init__(self, base_url: Optional[str] = None, token: Optional[str] = None):
        self.base_url = (base_url or FRACTAL_API_BASE).rstrip("/")
        self.token = token if token is not None else INTERNAL_TOKEN

    def _headers(self) -> Dict[str, str]:
        if self.token:
            return {"X-Fractal-Internal-Token": self.token}
        return {}

    async def case_evidence(self, case_id: str, evidence_type: Optional[str] = None) -> List[dict]:
        """Fetch normalized evidence rows for a case."""
        params: Dict[str, Any] = {}
        if evidence_type:
            params["type"] = evidence_type
        async with httpx.AsyncClient() as client:
            resp = await client.get(
                f"{self.base_url}/internal/v1/cases/{case_id}/evidence",
                params=params,
                headers=self._headers(),
                timeout=10.0,
            )
        resp.raise_for_status()
        return resp.json().get("evidence", [])

    async def webhook_events(self, case_id: str) -> List[dict]:
        """Fetch verified webhook history for a case."""
        async with httpx.AsyncClient() as client:
            resp = await client.get(
                f"{self.base_url}/internal/v1/cases/{case_id}/webhook-events",
                headers=self._headers(),
                timeout=10.0,
            )
        resp.raise_for_status()
        return resp.json().get("events", [])

    async def search_logs(self, case_id: str, window: str = "1h") -> List[dict]:
        """Search Elastic logs scoped to a case (read-only)."""
        async with httpx.AsyncClient() as client:
            resp = await client.get(
                f"{self.base_url}/internal/v1/telemetry/logs",
                params={"case_id": case_id, "window": window},
                headers=self._headers(),
                timeout=10.0,
            )
        resp.raise_for_status()
        return resp.json().get("hits", [])

    async def get_trace(self, trace_id: str) -> dict:
        """Fetch a single trace span tree (read-only)."""
        async with httpx.AsyncClient() as client:
            resp = await client.get(
                f"{self.base_url}/internal/v1/telemetry/traces/{trace_id}",
                headers=self._headers(),
                timeout=10.0,
            )
        resp.raise_for_status()
        return resp.json()

    async def handler_error_rate(self, merchant_id: str, window: str = "24h") -> dict:
        """Aggregate metric: per-merchant handler failure rate (read-only)."""
        async with httpx.AsyncClient() as client:
            resp = await client.get(
                f"{self.base_url}/internal/v1/telemetry/handler-error-rate",
                params={"merchant_id": merchant_id, "window": window},
                headers=self._headers(),
                timeout=10.0,
            )
        resp.raise_for_status()
        return resp.json()
