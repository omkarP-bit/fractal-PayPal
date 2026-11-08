# FRACTAL: Project Master Document

Shared payment-exception resolution platform. Version 2.0, 6 November 2026. Built for the PayPal and AI hackathon (Devpost). This is the consolidated backup context: the original architecture plus every decision and correction made since.

## 1. One-paragraph summary

FRACTAL connects a customer and a merchant through **one shared case ID** when a payment’s provider state, merchant order state and customer experience disagree (for example, PayPal says COMPLETED while the merchant order is still PAYMENT\_PENDING). It gathers evidence, lets an AI agent investigate, verifies the facts independently, applies deterministic policy through OPA, and executes only safe, idempotent, audited actions.

**Core principle:** AI investigates. Backend verifies. Policy governs. Backend executes. Everything is audited.

**Long-term vision:** a provider-agnostic exception-resolution layer between merchants and customers (PayPal first, then PhonePe and others). Goal is a real scalable project, not only a hackathon win.

**Hackathon requirement:** PayPal (Sandbox) and AI must both be meaningfully central. A working prototype and clear documentation are required.

## 2. What FRACTAL is and is not

FRACTAL is a payment-exception resolution and coordination platform. The payment provider stays authoritative for provider-side status.

It is **not** a payment gateway, a processor, a fraud system, a generic support chatbot, a PayPal replacement, or just a reconciliation dashboard. Keep the “not a fraud system” boundary: anomaly detection here is operational, not fraud scoring.

## 3. The problem (demo scenario)

1. Customer pays through PayPal checkout.
2. PayPal captures the payment and sends a webhook.
3. The merchant’s handler fails, so the order stays PAYMENT\_PENDING.
4. Customer sees success, merchant sees unpaid.

| System | State | Meaning |
| --- | --- | --- |
| PayPal | COMPLETED | Provider reports successful capture |
| Merchant | PAYMENT\_PENDING | Order never updated |
| FRACTAL | INVESTIGATING | Shared case open |

## 4. Decisions locked so far

| Topic | Decision | Reason |
| --- | --- | --- |
| Backend language | Go modular monolith | Concurrency handling for webhook bursts, replays, concurrent executions |
| PayPal access | Go calls PayPal REST APIs directly over HTTP | No Python adapter hop; fewer failure points; the component that executes money actions is the only one holding PayPal credentials |
| AI service | Python + LangGraph, single agent | Python is the natural ML/agent runtime; one agent with read-only tools is enough |
| Observability and evidence | Elastic (via OpenTelemetry) | Logs, traces and metrics in one place; sponsor tool; agent queries it read-only |
| Dashboards | Optional Grafana or Kibana on top | Not required for MVP |
| Policy | OPA + Rego | Deterministic, versioned, testable decisions |
| Database | PostgreSQL | State, evidence, decisions, audit, idempotency |
| Frontend | React + TypeScript, one app with role switch first | Two full portals only if time allows |
| Providers | PayPal only for the hackathon, behind a Go Provider interface | PayPal must be central; interface keeps PhonePe etc. possible later |
| ML anomaly layer | Phase 2, built last | Optional by design; core works without it |

## 5. Corrections to remember

1. **LGTM is not Elastic.** LGTM means Loki, Grafana, Tempo, Mimir. Elastic is Elasticsearch, Kibana, APM. Pick one backend. Chosen: Elastic, with Grafana optionally on top.
2. **OPA does not validate evidence.** It evaluates policy over whatever input it receives. If the agent’s claim goes in as a fact, OPA trusts it. So Go must independently re-fetch critical facts and build the OPA input itself. Agent output enters only as a recommendation.
3. **Go does not need an SDK.** PayPal REST is plain HTTPS and JSON, including webhook signature verification (verify-webhook-signature endpoint). Confirm current details in PayPal’s docs.
4. **A model score is a signal, not an authority.** Anomaly scores may only tighten decisions (force approval), never loosen them.

## 6. Trust boundary (the key design idea)

```text
 UNTRUSTED / ADVISORY                 TRUSTED / AUTHORITATIVE
 ┌──────────────────────┐             ┌────────────────────────────┐
 │ LangGraph agent      │             │ Go backend                 │
 │ - findings           │──advice───▶ │ - re-fetch PayPal state    │
 │ - recommendation     │             │ - re-read order state      │
 │ - suggested severity │             │ - verify webhooks          │
 │ - anomaly score      │             │ - build OPA input          │
 └──────────────────────┘             └──────────────┬─────────────┘
                                                     │ verified facts
                                                     ▼
                                      ┌────────────────────────────┐
                                      │ OPA / Rego PolicyGuard     │
                                      │ ALLOW | REQUIRE_APPROVAL   │
                                      │ WAIT  | BLOCK              │
                                      └──────────────┬─────────────┘
                                                     ▼
                                      Go executor (lock, idempotency,
                                      final freshness check, execute)
```

Answer to “what stops a hallucinating agent?”: its claims never reach the policy engine as facts.

## 7. System architecture

```text
 Customer UI ─┐                                   ┌─ Merchant UI
              ▼                                   ▼
        ┌───────────────── Go API (modular monolith) ─────────────────┐
        │ case mgmt │ authz │ evidence │ risk │ policy client │ exec │
        └───┬───────────┬──────────────┬───────────────┬──────────┬───┘
            │           │              │               │          │
   PayPal REST    PostgreSQL     Python/LangGraph       OPA     Merchant order
   + webhooks     (cases,        agent (read-only)   (Rego)    API (contract)
   (direct HTTP)  audit, idem)         │
            ▲                          ▼
            │                   Elastic (logs, traces, metrics)
            └────── Go emits telemetry via OpenTelemetry ──────┘
```

### Go modules (inside one binary)

- **api**: case endpoints, role-scoped projections, object-level authorization.
- **provider**: `Provider` interface (get transaction, verify webhook, refund, list webhook events). PayPal is implementation one.
- **webhook**: receiver, signature verification, dedup, persistence.
- **evidence**: normalizes provider, merchant and telemetry evidence with provenance.
- **risk**: derives operational risk from verified facts; reserves an `anomaly_score` input (null by default).
- **policy**: builds OPA input, calls OPA, stores decision with policy version and input hash.
- **executor**: case lock, idempotency check, fresh-state validation, merchant order update, refunds later.
- **audit**: append-only event log and portal notifications.

### Python service

One LangGraph agent. Read-only tools only: case evidence lookup, Elastic log/trace/metric search, webhook history. No PayPal credentials, no write tools. Output is structured JSON with evidence references.

## 8. End-to-end workflow

```text
1. Customer pays via PayPal Sandbox on mock merchant store
2. PayPal captures; webhooks go to merchant handler AND FRACTAL listener
   (merchant handler deliberately broken, order stays PENDING)
3. FRACTAL receiver: verify signature → dedupe on provider_event_id → store
4. Customer opens case, proves ownership → one shared case ID;
   merchant notified on the same case
5. Go gathers evidence: capture ref, provider status, webhook history,
   order state, plus Elastic logs/traces
6. LangGraph agent investigates (read-only) → structured findings
7. Go re-verifies critical facts (fresh PayPal fetch + order store)
   and builds OPA input; agent output = recommendation only
8. OPA decides: ALLOW | REQUIRE_APPROVAL | WAIT | BLOCK (reasons + policy version)
9. If ALLOW or approved: final freshness check → case lock →
   idempotency check → call merchant order API to reconcile
10. Audit event written; both portals updated
```

### Decision branches

```text
 OPA decision
   ├─ ALLOW             → Go validates fresh state → execute → audit
   ├─ REQUIRE_APPROVAL  → merchant portal approves/rejects → re-evaluate → execute or stop
   ├─ WAIT              → ask customer/merchant for missing evidence → loop to step 5
   └─ BLOCK             → record reason → escalate → audit
```

## 9. Policy design (PolicyGuard)

### Outcomes

| Decision | Meaning | Next step |
| --- | --- | --- |
| ALLOW | All mandatory checks pass | Backend executes after fresh validation |
| REQUIRE\_APPROVAL | Human authorization needed | Merchant approval workflow |
| WAIT | Critical evidence missing or stale | Collect more evidence |
| BLOCK | Mandatory rule violated | Stop, audit, escalate |

Mandatory security constraints override AI recommendations, merchant preferences and ordinary allow rules. Evaluation failure fails closed.

### MVP Rego rule set (small and visible)

- Webhook signature invalid → BLOCK.
- Provider status not COMPLETED → WAIT.
- Amount or currency mismatch → BLOCK automatic reconciliation (review).
- Evidence stale (fresh fetch older than threshold) → WAIT.
- Action already executed for this idempotency key → BLOCK.
- Actor lacks permission → BLOCK.
- Refund action → REQUIRE\_APPROVAL.
- Anomaly score above threshold (when present) → REQUIRE\_APPROVAL (tighten only).
- Policy version unknown → BLOCK.

### Policy layers

1. Global security and idempotency.
2. PayPal provider rules.
3. Merchant authorization and refund limits.
4. Operational risk.
5. Jurisdiction packs: **post-MVP**, only after legal review. Never claim worldwide compliance.

### OPA input shape (built by Go, not by the agent)

```json
{
  "case_id": "FR-10042",
  "actor": { "id": "USR-9", "role": "MERCHANT_ADMIN", "permissions": { "reconcile": true } },
  "merchant": { "id": "MER-501", "active": true },
  "payment": { "provider_status": "COMPLETED", "amount": "49.00", "currency": "USD", "fetched_at": "..." },
  "order": { "status": "PAYMENT_PENDING", "amount": "49.00", "currency": "USD" },
  "webhook": { "signature_verified": true },
  "action": { "type": "RECONCILE_MERCHANT_ORDER", "already_executed": false },
  "evidence": { "is_fresh": true },
  "risk": { "severity": "MEDIUM", "anomaly_score": null },
  "agent_recommendation": { "action": "RECONCILE_MERCHANT_ORDER", "confidence": 0.97 }
}
```

Confidence is informational. It never grants permission.

## 10. AI agent design

**Where AI genuinely adds value** (answers “why AI?”): messy customer free-text, correlating handler error logs, trace gaps and webhook delivery history, plain-language root cause, suggested merchant fix, and later cross-case pattern detection. The deterministic mismatch check is done by the backend, not the model.

**Output contract:**

```json
{
  "category": "PROVIDER_MERCHANT_STATE_MISMATCH",
  "root_cause_hypothesis": "Merchant webhook handler returned 500 after capture event",
  "evidence_refs": ["evt_123", "log_abc", "trace_def"],
  "missing_evidence": [],
  "recommendation": "RECONCILE_MERCHANT_ORDER",
  "suggested_severity": "MEDIUM",
  "customer_explanation": "...",
  "merchant_fix_suggestion": "..."
}
```

Rules: every claim cites an evidence reference; no write tools; no payment credentials; output validated against a schema before use.

## 11. Concurrency and idempotency (Go’s showcase)

Backed by database guarantees, not goroutines alone:

- Unique constraint on `provider_event_id` (webhook dedup).
- Unique `idempotency_key` on `action_executions`.
- Per-case lock via Postgres advisory lock or `SELECT ... FOR UPDATE`.
- Fresh provider-state fetch immediately before execution.
- Fail closed if PayPal is unreachable during final validation.

**Demo moment:** fire 50 concurrent replays of the same webhook. Exactly one business action executes.

## 12. Shared case model

### Case identity and access

- One case ID shared by customer and merchant; different permissions and projections.
- A case ID is never an access credential. Use object-level authorization.
- **Ownership proof for case creation:** customer must present something only the buyer has (order email plus a token from the merchant confirmation page), so nobody can open cases on other people’s payments.

### Lifecycle

```text
OPEN → COLLECTING_EVIDENCE → INVESTIGATING
   ├─→ AWAITING_MERCHANT_APPROVAL
   ├─→ AWAITING_CUSTOMER_EVIDENCE
   └─→ ACTION_PENDING → RESOLVED | ESCALATED | REJECTED → CLOSED
```

Provider status, merchant order status, refund status and case status are separate fields.

### Core tables (full schema is the next deliverable)

users, merchants, merchant\_customers, merchant\_orders, payment\_transactions, exception\_cases, webhook\_events, case\_evidence, case\_messages, investigations, policy\_decisions, approvals, action\_executions, audit\_events. Later: policy\_applicability, anomaly\_features.

### Visibility

| Information | Customer | Merchant |
| --- | --- | --- |
| Case, payment and order status | Yes | Yes |
| Investigation findings | Simplified | Detailed |
| Internal risk indicators | No | Authorized users |
| Policy decision | Simplified | Detailed |
| Audit history | Customer-relevant | Full, authorized |

## 13. Merchant integration contract

FRACTAL needs an authenticated merchant endpoint to act on orders. This is the real provider-agnostic integration point.

- `GET /orders/{id}`: read order status, amount, currency.
- `POST /orders/{id}/reconcile`: mark paid, requires an `Idempotency-Key` header and a FRACTAL signature.
- Optional: `POST /orders/{id}/refund-intent`.

For the hackathon, the mock merchant store implements this, plus a **break-the-handler toggle** that simulates webhook failure.

## 14. Webhook handling with PayPal

- FRACTAL registers its own webhook URL on the PayPal app (check PayPal’s limits on webhook URLs per app).
- Verify every event via PayPal’s verification flow; reject on failure and log as a security event.
- Dedupe, persist raw payload reference, process replay-safely.
- Use PayPal’s webhook event listing and resend features to support history and the replay demo (confirm availability in docs).
- Retrieve provider state before any critical operation.

## 15. Observability (Elastic)

- Go instruments logs, traces and metrics with OpenTelemetry and ships them to Elastic.
- Every case ID and event ID appears as a log and trace attribute so the agent can query by case.
- Agent tools: `search_logs(case_id)`, `get_trace(trace_id)`, `handler_error_rate(merchant_id, window)`. Read-only.
- Log from day one: webhook delivery latency, retry counts, handler error rates, time from capture to order update. These become ML features.

## 16. Optional ML anomaly layer (Phase 2)

**Scope:** operational anomaly detection (webhook latency and gaps, per-merchant handler failure rate, refund velocity, unusual timing), not fraud detection.

**Design:** VAE trained on normal behavior; reconstruction error and latent features feed a supervised classifier (hybrid). Benchmark against Isolation Forest and statistical baselines to prove it earns its complexity.

**Constraints:**

- Training data will be synthetic plus your own sandbox runs; state this honestly.
- Output is `anomaly_score` in the risk input. OPA uses it only to tighten decisions.
- Never turns BLOCK into ALLOW. The core works if the model is absent.
- Build order: event generator, baseline model, one dashboard moment (“this merchant’s webhook failures are 8x normal”), then VAE hybrid only if time remains.

## 17. Optional proactive reconciler

A background job that detects “capture completed, no order update after N minutes” and opens a case automatically. Small addition on stored webhook data; turns FRACTAL from reactive to proactive and supports Resolution Intelligence.

## 18. Demo script

1. Real PayPal Sandbox checkout on the mock store.
2. Handler broken by toggle; order stays PENDING.
3. Customer opens a case; one shared case ID appears in both views.
4. Agent shows evidence-backed diagnosis (logs, traces, webhook history).
5. OPA returns ALLOW with readable reasons; Go revalidates and reconciles; both views update.
6. Negative 1: invalid signature rejected.
7. Negative 2: amount mismatch → BLOCK; refund → REQUIRE\_APPROVAL.
8. Concurrency: 50 parallel replays, exactly one execution.
9. Audit timeline for the whole case.
10. Optional: anomaly dashboard (“5 complaints, 1 root cause”).

## 19. Test scenarios

Valid capture with matching amount; invalid signature; duplicate webhook; stale provider state; amount mismatch; currency mismatch; provider still pending; unauthorized case access; staff without permission; refund without approval; approval revoked before execution; concurrent execution; unknown policy version; PayPal unavailable at final validation; agent output with fabricated evidence reference (must be rejected by schema or verification).

**Success metrics:** zero duplicate actions under replay; zero reconciliations based solely on AI output; zero unverified webhooks accepted; zero unauthorized refunds; complete audit trail per executed action; correct customer/merchant visibility separation.

## 20. Build order

1. Case API contracts and Postgres schema (unique constraints for dedup and idempotency). **Next deliverable.**
2. Go: PayPal sandbox client, webhook receiver, verification, dedup.
3. Mock merchant store with break toggle and order-update endpoint.
4. OpenTelemetry into Elastic and read-only query tools.
5. LangGraph agent returning structured JSON.
6. Go verification layer, OPA input and Rego rules for four outcomes.
7. Executor with locking and idempotency; audit timeline.
8. Customer and merchant views (one app with role switch first).
9. Concurrency test.
10. Phase 2: anomaly layer, proactive reconciler, second portal polish.

**Cut order if time is short:** ML hybrid, second portal, analytics dashboard, jurisdiction packs. **Never cut:** webhook verification, dedup and replay test, OPA negative cases, audit trail.

## 21. Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Scope too large for deadline | Vertical slice first; cut order above |
| Agent hallucinates facts | Evidence refs required; Go re-verifies; schema validation |
| Duplicate webhooks or executions | DB constraints, idempotency keys, case lock |
| Stale state | Fresh fetch before execution; fail closed |
| Cross-user data exposure | Object-level authorization, role projections, ownership proof |
| Policy regressions | Versioned bundles, positive and negative test fixtures |
| Overstated compliance | Jurisdiction packs gated behind legal review |
| ML adds no value | Benchmark against simple baselines; keep optional |
| Sandbox limitations | Verify early what PayPal Sandbox supports (webhooks, refunds, disputes) |

## 22. Items to verify in PayPal docs early

- Webhook signature verification flow and required headers.
- Number of webhook URLs per app and the events available.
- Webhook event list and resend endpoints in Sandbox.
- Refund behavior in Sandbox.
- Current SDK support list (only relevant if you ever change the direct-REST decision).
- Rate limits and token handling for the Orders and Payments APIs.

## 23. Roadmap beyond the hackathon

- **Phase 1:** reliable exception resolution (this document’s MVP).
- **Phase 2:** merchant-approved refunds, notifications, policy administration, anomaly layer.
- **Phase 3:** more providers behind the Provider interface; validated jurisdiction packs; legal review process.
- **Phase 4:** Resolution Intelligence, cross-merchant detection of recurring integration failures.

## 24. Open questions

- Actual hackathon deadline and team size.
- Elastic deployment: Elastic Cloud trial or self-hosted for the demo.
- Frontend: single role-switch app or two portals.
- Policy for how long evidence is considered fresh.
- Notification channel (email, WebSocket, or in-app only for MVP).

## 25. Immediate next step

Draft the PostgreSQL schema and the shared case API contracts (endpoints, request and response shapes, role projections, error model, idempotency headers, webhook receiver contract and the merchant integration contract).

*Disclaimer: this is an engineering and product proposal, not legal advice or a compliance certification. Jurisdiction-specific policies must be reviewed by qualified legal and compliance professionals before production use.*
