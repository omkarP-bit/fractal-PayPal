# FRACTAL: Phase-wise Development Plan (Local Development)

Companion to the Project Master Document and the PolicyGuard, Schema and API Spec.
Scope of this file: how to build FRACTAL phase by phase on a local machine, what the web UI looks like for the customer and the merchant, and which references to take standards from.

**Core principle:** AI investigates. Backend verifies. Policy governs. Backend executes. Everything is audited.

---

## 0. Ground rules

1. **PayPal Sandbox only.** No live credentials anywhere on the machine.
2. **Local first.** Everything runs with one command through Docker Compose. Nothing needs a cloud account except PayPal Sandbox (and optionally an LLM API key).
3. **One vertical slice before breadth.** Finish the "customer charged, merchant order unpaid" flow end to end before adding anything else.
4. **Never cut:** webhook signature verification, deduplication, idempotent execution, OPA negative cases, audit trail.
5. **Secrets are never committed.** Use `.env` (git-ignored) plus `.env.example` with placeholders.
6. **Agent output is advisory.** Go re-verifies facts and builds the OPA input. The agent never holds PayPal credentials.

---

## 1. Technology stack and local tooling

| Area | Choice | Notes |
|---|---|---|
| Core backend | Go (modular monolith) | `net/http` with `chi`, `pgx`, `sqlc`, `golang-migrate`, `log/slog`, OpenTelemetry SDK |
| PayPal access | Direct REST from Go | No SDK, no adapter service |
| AI service | Python, FastAPI, LangGraph, Pydantic, httpx | One agent, read-only tools. LLM provider chosen by env var |
| Policy | OPA + Rego | Sidecar container loading a bundle from the `policies/` folder |
| Database | PostgreSQL | Schema from Spec Part B |
| Observability | OpenTelemetry to Elastic (Elasticsearch + Kibana) | Single-node local Elastic; needs a few GB of RAM |
| Frontend | React + TypeScript + Vite | TanStack Query, Tailwind, SSE for live updates |
| Mock merchant | Small Go or Node app | Real PayPal Sandbox checkout, breakable webhook handler |
| Webhook tunnel | `cloudflared` or `ngrok` | PayPal cannot reach `localhost`; a public tunnel URL is registered as the webhook URL |
| Orchestration | Docker Compose, Makefile | `make up`, `make test`, `make seed`, `make demo` |

### Local services

| Service | Port | Purpose |
|---|---|---|
| fractal-api (Go) | 8080 | Case API, webhook receiver, executor |
| merchant-store | 8090 | Mock merchant storefront and order API |
| investigator (Python) | 8100 | LangGraph agent |
| opa | 8181 | Policy decisions |
| postgres | 5432 | Primary database |
| elasticsearch | 9200 | Logs, traces, metrics |
| kibana | 5601 | Optional dashboards |
| web (Vite) | 5173 | Customer and merchant UI |

### Repository layout

```text
fractal/
  docker-compose.yml
  Makefile
  .env.example
  api/                    Go modular monolith
    cmd/fractal-api/
    internal/{cases,auth,provider,webhook,evidence,risk,policy,executor,audit,telemetry}
    migrations/
  investigator/           Python LangGraph service
    app/{graph,tools,schemas}
  merchant-store/         Mock merchant
  policies/               Rego, data, tests (see Spec Part A)
  web/                    React app
  docs/                   Master doc, spec, this plan, demo script
  scripts/                seed, replay, load test
```

---

## 2. Phase overview

| Phase | Name | Outcome |
|---|---|---|
| 1 | Foundation and local environment | Whole stack boots locally, PayPal Sandbox token works |
| 2 | Data layer and case core | Shared case API with role projections and audit |
| 3 | PayPal integration and webhooks | Verified, deduplicated webhook ingestion |
| 4 | Mock merchant store | Real checkout, breakable handler, merchant integration contract |
| 5 | Evidence pipeline and observability | Normalized evidence and case-scoped logs in Elastic |
| 6 | AI investigation | Validated, evidence-cited agent findings |
| 7 | PolicyGuard (OPA) | Four decision outcomes with explainable reasons |
| 8 | Executor and approvals | Locked, idempotent, approved actions |
| 9 | Web application | Customer and merchant UI for the full flow |
| 10 | Hardening, demo and documentation | Negative scenarios, concurrency proof, demo script |
| 11 | Anomaly detection (optional) | Operational anomaly score as a tighten-only signal |
| 12 | Proactive reconciler and resolution intelligence (optional) | Cases opened automatically, root-cause grouping |
| 13 | Beyond local | More providers, jurisdiction packs, production hardening |

Dependencies: 1 then 2 then 3 then 4. Phases 5 and 7 can start once 3 is done. Phase 6 needs 5. Phase 8 needs 7. Phase 9 can start after 2 and grows with each later phase.

---

## 3. Phases in detail

### Phase 1: Foundation and local environment

**Goal:** one command boots every service; PayPal Sandbox is reachable.

**Tasks**
- Create the monorepo layout above, Makefile, `.env.example`, `.gitignore`.
- Write `docker-compose.yml` for postgres, opa, elasticsearch, kibana, and stubs for api, merchant-store, investigator, web.
- Add health endpoints (`/healthz`, `/readyz`) to every service.
- Set up `golang-migrate`, `sqlc`, Go linting, Python linting (`ruff`), TypeScript linting.
- Create a PayPal Developer account, a Sandbox app, and two Sandbox accounts (business merchant and personal buyer).
- Fetch a Sandbox OAuth token from Go as a smoke test.
- Install a tunnel tool and document how to start it and where to paste the public URL.
- Add structured logging with a request ID in every service.

**Exit criteria**
- `make up` starts everything and all health checks pass.
- Go obtains a PayPal Sandbox access token.
- Empty migration runs and rolls back cleanly.

**References:** PayPal Developer docs (authentication, sandbox), Docker Compose docs, Elastic Docker install docs, OpenTelemetry docs.

---

### Phase 2: Data layer and case core

**Goal:** the shared case exists, with correct authorization and an audit trail.

**Tasks**
- Apply Spec Part B migrations (tenancy, payments, cases, evidence, decisions, approvals, executions, audit, outbox).
- Seed users: one customer, one merchant admin, one merchant staff, one platform admin.
- Dev auth: a small token issuer that signs JWTs with `sub`, `kind`, `merchant_id`, `role`, `permissions`.
- Authorization middleware plus **object-level checks on every case route**. Return 404 for non-parties.
- Case endpoints: create (with ownership proof), get, list, messages, timeline.
- Role projections: customer view versus merchant view (Spec C2).
- Idempotency-Key middleware and ETag / If-Match on cases.
- Audit service with hash chaining, computed inside the case lock.
- Transactional outbox and an SSE endpoint (`/cases/{id}/events`).
- Append-only triggers on evidence, decisions, audit.

**Exit criteria**
- A customer cannot read another customer's case (404).
- Customer and merchant see different projections of the same case ID.
- Audit chain verification script passes.
- Duplicate `POST /cases` with the same key returns the same case.

**References:** OWASP API Security Top 10 (object-level authorization first), OWASP ASVS (access control), RFC 9457 (problem details), PostgreSQL docs (row-level security, constraints).

---

### Phase 3: PayPal integration and webhooks

**Goal:** trustworthy provider events and provider state.

**Tasks**
- Define the Go `Provider` interface: `GetTransaction`, `VerifyWebhook`, `Refund`, `ListWebhookEvents`.
- Implement `PayPalProvider` with direct REST calls, token caching and retry with backoff.
- Implement order create, capture, and order and capture retrieval (used by the mock store and by evidence collection).
- Build the webhook receiver (Spec C6): raw body first, verify signature, reject and log invalid events, `INSERT ... ON CONFLICT DO NOTHING` for dedup, respond fast, process asynchronously.
- Register the tunnel URL as a webhook endpoint on the Sandbox app and store `webhook_id`.
- Persist `payment_transactions` from capture and webhook events.
- Write a replay script that resends a stored event to the receiver.

**Exit criteria**
- A real Sandbox payment produces a verified, stored webhook event.
- Tampered payload or wrong signature is rejected and logged in `security_events`.
- Replaying the same event 50 times stores one row and triggers one downstream job.

**References:** PayPal Webhooks and verification docs, RFC 3339, PostgreSQL docs (INSERT ON CONFLICT).

---

### Phase 4: Mock merchant store

**Goal:** a believable merchant whose order system can fail on demand.

**Tasks**
- Storefront with one or two products, PayPal checkout, order table (`PAYMENT_PENDING`, `PAID`, `CANCELLED`).
- Webhook handler that normally marks the order `PAID`, plus a **break toggle** (return 500, or drop the event, or delay it).
- Confirmation page that shows an order claim token (used as ownership proof when opening a case).
- Merchant integration contract (Spec C8): `GET /orders/{ref}`, `POST /orders/{ref}/reconcile`, HMAC signature check, timestamp window, idempotency.
- Log handler errors with the order reference (these become evidence in Elastic).

**Exit criteria**
- With the toggle off: pay, webhook arrives, order becomes `PAID`.
- With the toggle on: payment completes at PayPal, order stays `PAYMENT_PENDING`.
- Reconcile endpoint is idempotent and rejects bad signatures.

**References:** PayPal Orders and Payments API docs, RFC 2104 (HMAC), OWASP API Security Top 10.

---

### Phase 5: Evidence pipeline and observability

**Goal:** every case has normalized, provenance-tagged evidence and searchable telemetry.

**Tasks**
- Evidence collector in Go: fetch fresh PayPal state, fetch the merchant order, load webhook history; store each as immutable `case_evidence` with source, `fetched_at` and SHA-256.
- Instrument Go and the mock store with OpenTelemetry (logs, traces, metrics) exporting to Elastic.
- Put `case_id`, `event_id`, `merchant_id` on every log and span.
- Internal read-only telemetry endpoints (Spec C7): logs by case, trace by ID, handler error rate.
- Kibana saved search or dashboard for the demo (optional).

**Exit criteria**
- Opening a case triggers collection and the case has at least provider, order and webhook evidence.
- Searching Elastic by case ID returns the failed handler log and the trace.
- Evidence rows cannot be updated or deleted.

**References:** OpenTelemetry docs, Elastic docs (Elasticsearch, Kibana, APM and OTel ingest), PCI DSS Requirement 10 (logging and audit trails) for what to log.

---

### Phase 6: AI investigation

**Goal:** an evidence-cited diagnosis in plain language, kept advisory.

**Tasks**
- FastAPI service with `POST /v1/investigate`.
- Single LangGraph agent with read-only tools: case evidence, webhook events, Elastic log and trace search, handler error rate.
- Structured output with Pydantic (Spec C7 submission schema). Every claim cites evidence IDs.
- Treat customer free text as **untrusted input**: separate it from instructions, never let it change tool scope.
- Go-side validation: schema, evidence IDs exist and belong to the case, recommendation is in the action registry, length limits. Reject and audit failures.
- LLM provider behind an env var; timeouts, retries and a token budget.
- Build 8 to 10 seeded scenarios and a small eval script (expected category, expected cited evidence).

**Exit criteria**
- The seeded "handler 500" case yields the right category, a root cause, and valid evidence references.
- A submission with a fabricated evidence ID is rejected (422) and audited.
- The agent has no PayPal credentials and no write endpoint except submission.

**References:** LangGraph docs, OWASP Top 10 for LLM Applications (prompt injection, excessive agency), NIST AI RMF (risk framing), Pydantic docs.

---

### Phase 7: PolicyGuard (OPA)

**Goal:** deterministic, explainable decisions with four outcomes.

**Tasks**
- Implement the Spec Part A layout: `lib`, `guard`, rule modules (integrity, security, evidence, paypal, approvals, risk), data files (config, actions, merchants, packs, meta).
- Write fixtures and tests (Spec A10), including: valid reconcile, invalid signature, amount mismatch, stale evidence, refund without approval, unknown action, agent advice alone.
- CI-style checks: `opa test`, `opa check --strict`, `opa fmt`, advisory isolation grep, registry lint.
- Go input builder: fetch fresh facts, build the envelope, validate against JSON Schema, call OPA, store input, hash, decision and policy revision.
- Fail closed on OPA timeout or error.
- Policy applicability: a stub that returns `APPLICABLE` for provider and global packs and `REQUIRES_REVIEW` for any jurisdiction pack. Jurisdiction packs stay disabled.

**Exit criteria**
- All four outcomes (ALLOW, REQUIRE_APPROVAL, WAIT, BLOCK) are reachable end to end with readable reasons.
- Changing a threshold in data changes behavior without touching Rego code.
- Stopping OPA results in BLOCK, never ALLOW.

**References:** OPA docs (policy language, testing, bundles, decision logs, performance), Rego style guide, OWASP ASVS (authorization and logging), PayPal webhook docs (provider rules).

---

### Phase 8: Executor and approvals

**Goal:** safe execution that survives replays, races and crashes.

**Tasks**
- Executor sequence: per-case advisory lock, insert `action_executions` with `ON CONFLICT`, fresh PayPal and order check, call merchant reconcile with the same idempotency key, mark result, audit, outbox.
- Recovery job for rows stuck in `IN_PROGRESS`.
- Approvals: create on REQUIRE_APPROVAL, decision endpoint, re-evaluate with approval in `input.approvals`, bind to `params_sha256`, expiry, revoke, no self-approval.
- Refund path (P1) through `Provider.Refund` in PayPal Sandbox, always approval-gated.
- Case state machine enforcement (Spec C5).
- Concurrency test: 50 parallel webhook replays plus 20 parallel action requests; exactly one execution.

**Exit criteria**
- Exactly one reconcile happens under heavy parallel load.
- Revoked or expired approval blocks execution.
- A refund cannot execute without a valid, parameter-bound approval.

**References:** PostgreSQL docs (advisory locks, transactions), PayPal Payments API docs (refunds, idempotency via `PayPal-Request-Id`), OPA docs.

---

### Phase 9: Web application

**Goal:** a demo-ready web app for both roles. Wireframes are in section 4.

**Tasks**
- React app with role-aware routes: `/customer/*`, `/merchant/*`, `/demo` (control panel).
- Dev login with a role switcher (customer, merchant staff, merchant admin).
- Customer screens: case list, new case, case detail.
- Merchant screens: inbox, case detail (evidence, investigation, decision, actions), approval modal.
- SSE subscription that refetches the projection on events.
- Demo control panel: break or fix merchant handler, replay last webhook, send an invalid-signature webhook, fire concurrency test, reset data.
- Accessibility basics: labels on inputs, keyboard focus, visible status text (not color alone).

**Exit criteria**
- Full happy path works through the UI only.
- Customer never receives internal fields (verify in the network tab, not just the UI).
- Status changes appear in both views without a manual refresh.

**References:** WCAG 2.2 quick reference, OWASP ASVS (session and access control), MDN Server-Sent Events docs.

---

### Phase 10: Hardening, demo and documentation

**Goal:** the demo cannot embarrass you, and the project explains itself.

**Tasks**
- Automated negative suite covering all 15 scenarios from the Master Document (section 19).
- Security checklist pass against OWASP API Top 10 (object-level auth, broken auth, excessive data exposure, rate limiting, SSRF on any URL fetched, security misconfiguration).
- `make seed` creates users, merchant, provider account, products, policy bundle record.
- `make demo` runs the scripted flow and the negative cases.
- Write README: architecture, run steps, trust boundary diagram, limits.
- Write the demo script (below) and rehearse it with the control panel.
- Record known limitations honestly (synthetic data, Sandbox only, no jurisdiction packs enabled).

**Exit criteria**
- Fresh clone to working demo using only the README.
- Zero duplicate actions, zero unverified webhooks accepted, zero reconciliations from AI output alone, zero unauthorized refunds across the automated suite.

**References:** OWASP API Security Top 10, OWASP ASVS, ISO/IEC 27001 Annex A technological controls (as a checklist reference only).

---

### Phase 11: Anomaly detection (optional)

**Goal:** an operational anomaly score that can only make decisions stricter.

**Tasks**
- Feature log from earlier phases: webhook latency, retry counts, handler error rate, time from capture to order update, refund velocity.
- Synthetic event generator plus your own Sandbox runs; document clearly that data is synthetic.
- Baselines first: statistical thresholds and Isolation Forest.
- Then VAE (reconstruction error and latent features) feeding a supervised classifier. Benchmark against the baselines; keep it only if it earns its complexity.
- Write `anomaly_scores`, feed `risk.anomaly_score` into the OPA input. Only tightening rules may use it.
- One demo moment: "this merchant's webhook failures are 8x normal."

**Exit criteria**
- With the model disabled the system behaves identically (score is null).
- A high score can force REQUIRE_APPROVAL but can never turn BLOCK into ALLOW.

**References:** scikit-learn docs (Isolation Forest), PyTorch docs, original VAE paper (Kingma and Welling), NIST AI RMF.

---

### Phase 12: Proactive reconciler and resolution intelligence (optional)

**Tasks**
- Background job: captured payment with no order update after N minutes opens a case with `opened_via = AUTO_RECONCILER`.
- Group cases by root cause per merchant ("5 complaints, 1 root cause") and show on a merchant insights page.

**Exit criteria:** an auto-opened case follows the same pipeline and the same policy gates as a customer-opened one.

---

### Phase 13: Beyond local

- Additional providers behind the `Provider` interface (for example PhonePe) with their own policy module and the same gate names.
- Jurisdiction policy packs, each enabled only after legal review (see compliance section).
- Production hardening: secrets manager, key rotation, rate limiting, backups, SLOs, deployment, signed policy bundles, PCI scope assessment.
- Container orchestration only if scale justifies it.

---

## 4. Web UI wireframes

Two roles, one web application. The same case ID opens a different server-built projection per role. Client code never filters data.

### 4.1 Mock store: checkout and confirmation

```text
+--------------------------------------------------------------+
| ACME STORE                                      Cart (1)     |
+--------------------------------------------------------------+
|  [product image]   Wireless Mouse        49.00 USD           |
|                    [ PayPal Checkout ]                       |
+--------------------------------------------------------------+

Confirmation page
+--------------------------------------------------------------+
| Thank you. Order ORDER-789                                   |
| Status: PAYMENT PENDING  (payment is being confirmed)        |
|                                                              |
| Problem with this order?                                     |
| Your claim code:  K7Q2-M9XD      [ Report a payment issue ]  |
+--------------------------------------------------------------+
```

The claim code is the ownership proof used when opening a FRACTAL case.

### 4.2 Customer: open a case

```text
+--------------------------------------------------------------+
| FRACTAL   [Customer]                      buyer@example.com  |
+--------------------------------------------------------------+
| Report a payment issue                                       |
|                                                              |
| Merchant        [ Acme Store                       v ]       |
| Order ref       [ ORDER-789                          ]       |
| Email           [ buyer@example.com                  ]       |
| Claim code      [ K7Q2-M9XD                          ]       |
| What happened?  [ I was charged but the store says   ]       |
|                 [ unpaid.                            ]       |
|                                                              |
|                              [ Cancel ]  [ Open case ]       |
+--------------------------------------------------------------+
If a live case already exists for this payment, the customer is taken to it.
```

### 4.3 Customer: case detail

```text
+----------------------------------------------------------------------+
| FRACTAL [Customer]      Case FR-10042   [Investigating]    buyer@... |
+----------------------------------------------------------------------+
| Progress: Opened > Evidence collected > [Investigating] > Resolution |
+--------------------------------------------+-------------------------+
| WHAT IS HAPPENING                          | CASE TIMELINE           |
| Your payment was completed, but the        | 10:02 Case opened       |
| merchant order has not updated yet. The    | 10:02 Payment confirmed |
| merchant has been notified.                | 10:03 Merchant notified |
| Next step: nothing needed. Do not pay      | 10:04 Investigation     |
| again.                                     |       started           |
+--------------------------------------------+-------------------------+
| PAYMENT AND ORDER                          | HIDDEN FROM CUSTOMERS   |
| Payment (PayPal)   Completed, 49.00 USD    | [ internal risk data  ] |
| Merchant order     Payment pending         | [ policy rules / raw  ] |
| Case               Investigating           | [ AI and server logs  ] |
+--------------------------------------------+ [ provider IDs        ] |
| CONVERSATION                               +-------------------------+
| You 10:02       I was charged but...       | YOUR ACCESS             |
| Merchant 10:06  We are checking...         | Can: read, message,     |
| [ write a message...            ] [Send]   |      add evidence       |
| [ Add evidence ]                           | Cannot: approve, refund |
+--------------------------------------------+-------------------------+
```

Customer sees: case status, payment and order status, a plain-language summary, customer-visible timeline events, shared messages. Customer never sees: provider IDs, logs, traces, risk indicators, raw policy input, merchant-only notes, internal audit events.

### 4.4 Merchant: inbox

```text
+----------------------------------------------------------------------+
| FRACTAL [Merchant]       Cases                 ops@acme (Admin)  v   |
+----------------------------------------------------------------------+
| Filter: [All v] [Needs approval] [Investigating] [Escalated]         |
+----------------------------------------------------------------------+
| Case      | Customer      | Order      | Type            | Status    |
| FR-10042  | b***@ex.com   | ORDER-789  | State mismatch  | Investig. |
| FR-10040  | a***@ex.com   | ORDER-775  | Refund request  | Approval  |
+----------------------------------------------------------------------+
```

### 4.5 Merchant: case detail

```text
+----------------------------------------------------------------------+
| FRACTAL [Merchant]  Case FR-10042  [Investigating]   ops@acme (Admin)|
+--------------------------------------------+-------------------------+
| SHARED STATE                               | EVIDENCE AND LOGS       |
| PayPal PAYPAL-ORDER-123 / CAPTURE-456      | E-1 PayPal transaction  |
|   COMPLETED  49.00 USD   fetched 10:03:40  | E-2 Order snapshot      |
| Order ORDER-789   PAYMENT_PENDING          | E-3 Webhook WH-8812 ok  |
+--------------------------------------------+ E-4 Handler log HTTP500 |
| AI INVESTIGATION (advisory)                | Trace: no order span    |
| Category: state mismatch                   +-------------------------+
| Root cause: handler returned 500 after     | AUDIT TIMELINE (full)   |
|   capture event                            | 10:02 Case opened  [C]  |
| Cited: E-1 E-2 E-3 E-4 (verified)          | 10:04 Agent run    [M]  |
| Severity: Medium                           | 10:05 Decision     [M]  |
+--------------------------------------------+ 10:06 Reconciled   [C]  |
| POLICYGUARD DECISION   [ ALLOW ]           | [C] customer-visible    |
| rev 2026-11-06.1                           | [M] merchant-only       |
| PASS SEC-101 webhook signature verified    +-------------------------+
| PASS EVD-101 provider state fresh          | YOUR ACCESS (Admin)     |
| PASS PP-101  provider COMPLETED            | Can: reconcile, approve |
| PASS PP-102  amount and currency match     |   others' requests, see |
| PASS APR-102 no approval required          |   risk score            |
+--------------------------------------------+ Cannot: edit policy,    |
| ACTIONS                                    |   see other merchants   |
| [ Reconcile order ] [ Refund (approval) ]  +-------------------------+
+--------------------------------------------+
```

Merchant sees: provider IDs, detailed findings, case-scoped evidence and logs, policy rules and reasons, merchant-only audit events, risk indicators (admin only). Merchant staff do not see the risk score and cannot approve.

### 4.6 Merchant: approval modal and blocked decision

```text
+--------------------------------------------------+
| Approve refund for FR-10040                      |
| Amount 49.00 USD    Requested by: staff@acme     |
| Policy: REQUIRE_APPROVAL (APR-001)               |
| Approval is bound to these exact parameters and  |
| expires in 15 minutes.                           |
| Reason [ Verified with customer          ]       |
|                  [ Reject ]  [ Approve ]         |
+--------------------------------------------------+

+--------------------------------------------------+
| Decision: [ BLOCK ]                              |
| PP-002 Amount mismatch (provider 49.00 vs order  |
|        50.00). Nothing was executed.             |
| Audit event written. Case escalated.             |
+--------------------------------------------------+
```

### 4.7 Demo control panel (`/demo`)

```text
+--------------------------------------------------------------+
| DEMO CONTROLS (local only)                                   |
| Merchant handler:  ( ) Working   (x) Broken                  |
| [ Replay last webhook ]  [ Send invalid-signature webhook ]  |
| [ Fire 50 concurrent replays ]  Result: 1 executed, 49 dup  |
| [ Seed amount mismatch ]  [ Reset demo data ]                |
+--------------------------------------------------------------+
```

### 4.8 Who sees what

| Item | Customer | Merchant staff | Merchant admin | Platform admin |
|---|---|---|---|---|
| Case, payment, order status | Yes | Yes | Yes | Read for audit |
| Provider order and capture IDs | No | Yes | Yes | Yes |
| AI findings | Simplified | Detailed | Detailed | Detailed |
| Evidence, Elastic logs and traces | No | Case-scoped | Case-scoped | Yes |
| Policy decision | Plain reason | Rules and findings | Rules and findings | Yes |
| Raw policy input, anomaly score | No | No | Yes | Yes |
| Audit timeline | Customer-visible | Customer and merchant-only | Customer and merchant-only | Full, with internal |
| Messages | Shared | Shared and merchant-only | Shared and merchant-only | No |
| Propose reconcile | No | With permission | With permission | No |
| Approve refund | No | No | Yes, not own request | No |
| Edit policy bundles | No | No | No | Yes |

---

## 5. Demo script

1. Start the stack (`make up`, tunnel running, webhook registered).
2. On the mock store, set the handler to **Broken**.
3. Pay through PayPal Sandbox. Show the order stuck at `PAYMENT_PENDING`.
4. As the customer, open a case using the claim code. Show the single case ID.
5. Switch to the merchant. Show evidence, Elastic log (HTTP 500), AI root cause with cited evidence.
6. Show the PolicyGuard decision with rule IDs and policy revision. Reconcile. Show both views updating.
7. Negative 1: send an invalid-signature webhook. Rejected, logged as a security event.
8. Negative 2: seed an amount mismatch. Decision is BLOCK with the reason shown.
9. Negative 3: request a refund as staff. REQUIRE_APPROVAL. Approve as admin. Executed in Sandbox.
10. Concurrency: fire 50 concurrent replays. One execution.
11. Show the audit timeline and hash-chain verification.
12. Optional: anomaly or "5 complaints, 1 root cause" insight.

---

## 6. Testing strategy

| Layer | What | Tooling |
|---|---|---|
| Policy | Positive and negative fixtures per rule, advisory isolation, registry lint | `opa test`, `opa check`, grep in CI |
| Go unit | Projections, authorization, state machine, signature verification, idempotency | `go test` with table tests |
| Go integration | Real Postgres: dedup, advisory lock, append-only triggers, audit chain | `testcontainers` or Compose |
| Concurrency | 50 webhook replays, 20 action requests | Go test plus `scripts/` load script |
| Agent | Seeded scenarios, fabricated-reference rejection, prompt-injection attempts in customer text | `pytest` |
| End to end | Happy path and the 15 negative scenarios | Playwright against the local stack |

---

## 7. Compliance and standards references

FRACTAL does not claim worldwide compliance. These are sources to take requirements from. Verify the current version of each before encoding a rule. Legal applicability depends on payment rail, regulated entity, transaction type and jurisdiction. Every jurisdiction pack needs qualified legal and compliance review before it is enabled.

### 7.1 What to use directly (engineering controls)

| Area | Reference | Where it applies in FRACTAL | Phase |
|---|---|---|---|
| PayPal webhooks and verification | https://developer.paypal.com/api/rest/webhooks/ and https://developer.paypal.com/api/rest/webhooks/rest/ | Signature verification, event handling, resend | 3 |
| PayPal REST APIs (Orders, Payments, refunds) | https://developer.paypal.com/docs/api/overview/ | Provider interface, idempotency header for refunds | 3, 8 |
| API security | https://owasp.org/www-project-api-security/ (OWASP API Security Top 10) | Object-level authorization, auth, data exposure, rate limiting, SSRF | 2, 10 |
| Application security verification | https://owasp.org/www-project-application-security-verification-standard/ (OWASP ASVS) | Access control, logging, input validation checklists | 2, 7, 10 |
| LLM application risks | https://owasp.org/www-project-top-10-for-large-language-model-applications/ | Prompt injection, excessive agency, output handling | 6 |
| AI risk framing | https://www.nist.gov/itl/ai-risk-management-framework | Documenting AI limits and human oversight | 6, 11 |
| Open Policy Agent | https://www.openpolicyagent.org/docs/latest/ | Rego language, testing, bundles, decision logs | 7 |
| Problem details for APIs | https://www.rfc-editor.org/rfc/rfc9457 | Error format | 2 |
| Timestamps | https://www.rfc-editor.org/rfc/rfc3339 | All time fields | 2 |
| HMAC | https://www.rfc-editor.org/rfc/rfc2104 | Merchant integration signing | 4 |
| PostgreSQL locking and RLS | https://www.postgresql.org/docs/current/explicit-locking.html and https://www.postgresql.org/docs/current/ddl-rowsecurity.html | Advisory locks, row-level security | 2, 8 |
| OpenTelemetry | https://opentelemetry.io/docs/ | Logs, traces, metrics instrumentation | 5 |
| Elastic | https://www.elastic.co/docs | Ingest, search, Kibana | 5 |
| LangGraph | https://langchain-ai.github.io/langgraph/ | Agent graph and tools | 6 |

### 7.2 Security standards to align with

| Standard | Reference | How FRACTAL uses it |
|---|---|---|
| PCI DSS (current version published by the PCI SSC) | https://www.pcisecuritystandards.org/document_library/ | Keep card data out of FRACTAL (PayPal handles checkout). Use Requirement 3 (stored data), 6 (secure development), 7 and 8 (access control and authentication), 10 (logging and monitoring) as design inputs. Assess actual PCI scope before production |
| ISO/IEC 27001 | https://www.iso.org/standard/27001 | Reference for access management, logging, supplier security, incident handling. Use Annex A technological controls as a checklist, not a certification claim |

### 7.3 Jurisdiction policy packs (post-MVP, disabled until legally reviewed)

| Jurisdiction | Reference | Notes for pack design |
|---|---|---|
| United States, Regulation E | https://www.consumerfinance.gov/rules-policy/regulations/1005/ and https://www.ecfr.gov/current/title-12/chapter-X/part-1005 | Error resolution procedures are in section 1005.11. Applies only to covered electronic fund transfers and covered entities. Do not apply to every PayPal transaction |
| EU/EEA, PSD2 | https://eur-lex.europa.eu/eli/dir/2015/2366/oj | Check the articles on notification and refunds for unauthorized or incorrectly executed payments and on provider liability (for example Articles 71, 73, 74 and 89) against the current consolidated text and successor legislation |
| EU/EEA, GDPR | https://eur-lex.europa.eu/eli/reg/2016/679/oj | Personal data handling in cases, messages and evidence |
| United Kingdom, Payment Systems Regulator | https://www.psr.org.uk/ | APP scam reimbursement applies to defined eligible payment types only. Not a general PayPal refund rule |
| India, RBI | https://www.rbi.org.in/ | Look up the RBI circular on harmonisation of turnaround time and customer compensation for failed transactions on authorised payment systems, and its later amendments. Relevant if you add UPI or PhonePe-style providers |
| India, data protection | https://www.meity.gov.in/ | Digital Personal Data Protection Act and rules. Relevant to storing customer data |

### 7.4 How to turn a reference into a policy rule

1. Write down the clause, its source URL, version and your reading of who it applies to.
2. Decide applicability inputs (payment rail, regulated entity, transaction type). If they are unknown, the applicability result is `REQUIRES_REVIEW`.
3. Encode the obligation as a rule module with a rule ID that cites the clause (for example `REGE-001`).
4. Add positive and negative fixtures.
5. Get legal and compliance review. Only then set `enabled: true` and `legal_review: APPROVED` in `packs.json`.
6. Record the pack version in `policy_decisions` so every decision is traceable to the exact text you encoded.

---

## 8. Definition of done (every phase)

- Code merged with unit tests for new logic.
- Migrations reversible.
- New endpoints covered by authorization tests (right role, wrong role, non-party).
- New actions registered in `actions.json` with required gates and policy tests.
- Logs and traces carry `case_id` and request ID.
- README section updated for how to run and test the phase.
- No secrets in the repository.

---

## 9. Risks to watch while building

| Risk | Response |
|---|---|
| PayPal cannot reach localhost | Use a tunnel; re-register the webhook URL when the tunnel URL changes |
| Elastic uses too much local memory | Single node, capped heap; fall back to a trial Elastic Cloud deployment if needed |
| Sandbox behavior differs from live | Verify early: webhook delivery, refund behavior, event resend; note limits in the README |
| Agent output looks confident but is wrong | Required evidence references, Go re-verification, OPA never reads advisory fields |
| Scope creep into ML or packs | Phases 11 to 13 are optional until Phase 10 exit criteria are met |
| Duplicate execution under load | Database constraints plus advisory lock plus the 50-replay test |

---

*This is an engineering plan, not legal advice or a compliance certification. Regulatory policy packs must be validated by qualified legal and compliance professionals before production use.*
