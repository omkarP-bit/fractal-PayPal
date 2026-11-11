"""Optional LLM pass for the investigation node.

Uses any OpenAI-compatible chat-completions endpoint. When no key is
configured the caller falls back to the deterministic graph, so the platform
works without an LLM. Output is advisory: Go re-verifies every reference.
"""

import json
import os
from typing import List, Optional

import httpx


SYSTEM_PROMPT = """You are a payment exception investigator. You receive verified
evidence for one case: a provider transaction, a merchant order snapshot and
verified webhook events. Explain the discrepancy and recommend one action.

Return ONLY a JSON object with these keys:
  category (string, <=100 chars)
  root_cause_hypothesis (string, <=2000 chars)
  evidence_ids (array of evidence ids you actually used, copied exactly)
  missing_evidence (array of strings)
  recommendation (one of: RECONCILE_MERCHANT_ORDER, REFUND_PAYMENT, NO_ACTION)
  suggested_severity (LOW, MEDIUM, HIGH or CRITICAL)
  customer_explanation (string, <=2000 chars, no technical jargon)
  merchant_fix_suggestion (string, <=2000 chars)

Never invent evidence ids. Never claim an action was executed; you only advise.
"""


def _compact(value, limit: int = 16000) -> str:
    text = json.dumps(value, default=str, separators=(",", ":"))
    return text[:limit]


async def investigate_with_llm(
    evidence: List[dict],
    events: List[dict],
    logs: List[dict],
    case_type: str,
    customer_description: str,
) -> Optional[dict]:
    api_key = os.getenv("LLM_API_KEY", "").strip()
    if not api_key:
        return None
    base = os.getenv("LLM_BASE_URL", "https://api.openai.com/v1").rstrip("/")
    model = os.getenv("LLM_MODEL", "gpt-4o-mini").strip() or "gpt-4o-mini"

    payload = {
        "case_type": case_type,
        "customer_description": customer_description,
        "evidence": evidence,
        "webhook_events": events,
        "logs": logs[:20],
    }
    body = {
        "model": model,
        "temperature": 0,
        "response_format": {"type": "json_object"},
        "messages": [
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user", "content": _compact(payload)},
        ],
    }

    try:
        async with httpx.AsyncClient(timeout=25.0) as client:
            resp = await client.post(
                f"{base}/chat/completions",
                headers={"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"},
                json=body,
            )
        resp.raise_for_status()
        content = resp.json()["choices"][0]["message"]["content"]
        data = json.loads(content)
    except Exception:
        return None

    if not isinstance(data, dict):
        return None
    data.setdefault("agent_version", "investigator-llm/1.0")
    data.setdefault("model_id", model)
    return data
