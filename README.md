# FRACTAL

**Payments should not become mysteries.**

FRACTAL is a shared payment-exception resolution platform. When a payment's provider state, the merchant's order state and the customer's experience disagree, FRACTAL gives the customer and the merchant **one shared case**, investigates the evidence with AI, verifies the facts independently, applies deterministic policy, and coordinates a safe, traceable resolution.

> **AI investigates. Backend verifies. Policy governs. Backend executes. Everything is audited.**

Built for the *Build What's Next with PayPal and AI* hackathon. PayPal Sandbox is the primary provider.

**Status:** in development. The commands and endpoints below describe the target local workflow. See [`docs/FRACTAL_Development_Plan.md`](docs/FRACTAL_Development_Plan.md) for phase-by-phase progress.

---

## The problem

1. A customer pays through PayPal checkout.
2. PayPal captures the payment and sends a webhook.
3. The merchant's webhook handler fails, so the order stays `PAYMENT_PENDING`.
4. The customer sees "paid". The merchant sees "unpaid". Nobody shares the same facts.

| System | State | Meaning |
|---|---|---|
| PayPal | `COMPLETED` | Provider reports a successful capture |
| Merchant | `PAYMENT_PENDING` | Order was never updated |
| FRACTAL | `INVESTIGATING` | One shared case is open |

## What FRACTAL does

- **One shared case ID** for customer and merchant, with role-specific views of the same case.
- **Verified evidence:** PayPal state, merchant order state, verified webhook history, and case-scoped logs and traces from Elastic, all with provenance.
- **AI investigation:** a single LangGraph agent with read-only tools produces an evidence-cited diagnosis and a plain-language explanation.
- **Independent verification:** the Go backend re-fetches critical facts and builds the policy input itself. Agent claims are advisory only.
- **PolicyGuard (OPA):** deterministic decisions: `ALLOW`, `REQUIRE_APPROVAL`, `WAIT`, `BLOCK`, each with rule IDs and a policy revision.
- **Safe execution:** per-case locking, idempotency keys, fresh-state checks and human approval for sensitive actions.
- **Audit trail:** append-only, hash-chained events with customer-visible and merchant-only visibility.

## What FRACTAL is not

Not a payment gateway or processor, not a fraud system, not a generic support chatbot, and not a replacement for PayPal. The provider stays authoritative for provider-side transaction status.

---

## Architecture

```mermaid
flowchart TB
    C[Customer UI] --> API
    M[Merchant UI] --> API
    PP[PayPal REST and webhooks] --> WH[Webhook receiver]
    WH --> N[Verify, dedupe, store]
    N --> DB[(PostgreSQL)]
    API[Go API: cases, authz, evidence, executor] <--> DB
    API -->|read-only evidence| AG[Python LangGraph agent]
    AG -->|advisory findings| API
    AG --> ES[(Elastic: logs, traces, metrics)]
    API -->|verified facts| OPA[OPA PolicyGuard]
    OPA -->|ALLOW| EX[Executor]
    OPA -->|REQUIRE_APPROVAL| HA[Merchant approval]
    HA --> EX
    OPA -->|WAIT| API
    OPA -->|BLOCK| AUD[Audit]
    EX -->|fresh state check| PP
    EX --> MS[Merchant order API]
    EX --> AUD
    AUD --> C
    AUD --> M
```

### Trust boundary

```text
 ADVISORY (untrusted)                 AUTHORITATIVE (trusted)
 +----------------------+            +-----------------------------+
 | LangGraph agent      |  advice    | Go backend                  |
 | - findings           | ---------> | - re-fetch PayPal state     |
 | - recommendation     |            | - re-read order state       |
 | - suggested severity |            | - verify webhooks           |
 +----------------------+            | - build OPA input           |
                                     +--------------+--------------+
                                                    | verified facts
                                                    v
                                     +-----------------------------+
                                     | OPA / Rego PolicyGuard      |
                                     | ALLOW | REQUIRE_APPROVAL    |
                                     | WAIT  | BLOCK               |
                                     +--------------+--------------+
                                                    v
                                     Executor: lock, idempotency,
                                     fresh check, execute, audit
```

A hallucinating agent cannot cause an action because its claims never reach the policy engine as facts, and the agent holds no payment credentials.

### Tech stack

| Area | Technology |
|---|---|
| Core backend | Go modular monolith, calls PayPal REST directly (no SDK, no adapter service) |
| AI service | Python, FastAPI, LangGraph, Pydantic |
| Policy | Open Policy Agent, Rego |
| Database | PostgreSQL |
| Observability | OpenTelemetry to Elastic (Elasticsearch + live Kibana dashboards) |
| Frontend | React, TypeScript, Vite |
| Mock merchant | Small storefront with a breakable webhook handler |
| Local orchestration | Docker Compose, Makefile |

---

## Quick start (local)

### Prerequisites

- Docker and Docker Compose
- Go, Node.js and Python (only needed if you run services outside containers)
- A PayPal Developer account with one Sandbox app. The deployment uses it for one-click onboarding; merchants do not
  create apps or copy credentials.
- For local development only, a tunnel tool (`cloudflared` or `ngrok`) so PayPal can reach the local webhook receiver. A
  public HTTPS deployment needs no tunnel.
- An API key for your chosen LLM provider (set through environment variables)
- Several GB of free RAM for single-node Elasticsearch

### 1. Configure

```bash
git clone <your-repo-url> fractal
cd fractal
cp .env.example .env
```

Fill in `.env`:

| Variable | Purpose |
|---|---|
| `PAYPAL_CLIENT_ID`, `PAYPAL_CLIENT_SECRET` | Deployment PayPal sandbox app. Powers one-click managed onboarding and remains the legacy fallback. |
| `PAYPAL_WEBHOOK_ID` | Optional legacy fallback used only by the seeded demo account |
| `PAYPAL_ENV` | `sandbox` (default and only supported value for local use) |
| `FRACTAL_ENCRYPTION_KEY` | Application key that seals merchant PayPal secrets at rest. Use a random 32-byte value in production. |
| `FRACTAL_PUBLIC_URL` | Public origin used to build webhook URLs, for example `https://fractal.onrender.com` |
| `PUBLIC_TUNNEL_URL` | Local fallback origin when you run FRACTAL behind a tunnel; must be HTTPS |
| `KIBANA_URL` | Live Kibana dashboard URL shown on `/demo` |
| `DATABASE_URL` | PostgreSQL connection string |
| `OPA_URL` | OPA server address |
| `ELASTIC_URL`, `ELASTIC_API_KEY` | Elastic endpoint and credentials |
| `LLM_PROVIDER`, `LLM_API_KEY`, `LLM_MODEL` | Agent model configuration |
| `MERCHANT_SIGNING_SECRET` | HMAC secret shared with the mock merchant |
| `JWT_DEV_SECRET` | Dev token signing secret |

Never commit `.env`. Use Sandbox credentials only.

### 2. Start the stack

```bash
make up        # postgres, opa, elasticsearch, kibana, api, investigator, merchant-store, web
make migrate-up # apply database migrations
make seed      # users, merchant, provider account, products, policy bundle record
```

### 3. Onboard a merchant (self-service, one click)

Open <http://localhost:5173/signup> and complete the two steps:

1. **Business and region.** Choose a country such as India or the USA. The region fixes the settlement currency, so
   amounts and currencies cannot silently default to USD. India is treated as cross-border only: the merchant prices
   export orders in USD, EUR, GBP, AUD, CAD, SGD or AED because PayPal does not support domestic INR checkout.
2. **Connect PayPal.** Choose **Connect with FRACTAL sandbox app**. FRACTAL uses the deployment's
   `PAYPAL_CLIENT_ID`/`PAYPAL_CLIENT_SECRET`, gets an access token, registers
   `https://<app>/webhooks/paypal/<provider_account_id>` with PayPal, and stores the returned webhook ID. The merchant
   does not create a PayPal app, copy credentials, or run a tunnel.

   **Advanced.** Expand **I have my own PayPal app** to paste a merchant-owned sandbox app instead. FRACTAL encrypts
   that secret with `FRACTAL_ENCRYPTION_KEY`. The Client ID is shown after connecting; the Secret is never returned.
   **Update credentials** rotates it.

PayPal only accepts **public HTTPS** webhook URLs. The deployment sets `FRACTAL_PUBLIC_URL` to its public origin (for
example the Render URL); local development can set `PUBLIC_TUNNEL_URL` to the HTTPS URL printed by
`cloudflared tunnel --url http://localhost:5173`. This is an operator setting, not a merchant step. Without either one,
onboarding returns a clear "Public webhook URL required" message instead of a generic PayPal error.

Managed accounts store **no merchant PayPal secret**: `credentials_ref` is `platform`, `external_account_id` is
`PLATFORM:<merchant_id>`, and the receiver verifies signatures with the deployment app credentials. Merchant-owned apps
keep their encrypted secret. In both cases the secret is never returned to the browser or written to logs, and each
merchant still has its own provider account and webhook URL. Rotating the deployment's PayPal app invalidates managed
webhooks, so re-register each merchant after changing `PAYPAL_CLIENT_ID`/`PAYPAL_CLIENT_SECRET`.

**Sandbox webhook limit.** PayPal allows 10 webhooks per sandbox app, which is enough for the current testing cohort.
When the limit is reached, onboarding returns a clear conflict and asks you to remove an unused webhook in the PayPal
Developer Dashboard before retrying. Production onboarding would use PayPal Partner/Connect onboarding instead of a
shared sandbox app.

**Why not email or a message queue?** PayPal does not expose an API to create REST apps or read their secrets, and email
is not a safe credential channel, so FRACTAL never asks merchants to email credentials. The webhook receiver is already
FRACTAL's own endpoint; verified events are stored in PostgreSQL and drained by a DB-backed worker with
`FOR UPDATE SKIP LOCKED`, so no external message queue is needed. A single shared webhook URL was rejected because the
receiver uses the URL to resolve the merchant and its signing credentials; a shared URL would make attribution and
signature verification ambiguous.

Existing merchants can reconnect or recover a deleted webhook from **PayPal setup** in the merchant navigation.

### 4. Legacy local tunnel (seeded demo account only)

Self-service onboarding is the normal path. For the seeded Acme Store, the old tunnel workflow still works:

```bash
cloudflared tunnel --url http://localhost:8080   # or: ngrok http 8080
```

The seeded provider account ID is `00000000-0000-0000-0000-0000000000c2` (`provider_accounts.id`, **not**
`merchants.id`). Its `PAYPAL_WEBHOOK_ID` fallback exists only for backwards compatibility.

### 5. Open the apps

| App | URL |
|---|---|
| FRACTAL web (customer and merchant) | http://localhost:5173 |
| Merchant signup | http://localhost:5173/signup |
| Mock merchant store | http://localhost:8090 |
| Demo control panel (live webhook telemetry) | http://localhost:5173/demo |
| Kibana (live logs and traces) | http://localhost:5601 |

Seeded dev users (change in `scripts/seed`): one customer, one merchant admin, one merchant staff, one platform admin. Use the role switcher on the login page.

---

## Run the demo

To test a newly onboarded merchant instead of the seeded Acme Store, point the mock store at that merchant and restart
it:

```bash
MERCHANT_ID=<merchant-id> docker compose up -d --force-recreate merchant-store
```

Then open <http://localhost:8090> and use the form, or share the checkout link
`http://localhost:8090/pay?email=buyer@example.com`. Each visit creates a fresh order and redirects to PayPal Sandbox;
the order reference and single-use claim code appear on the confirmation page after payment. Use a customer email that
has a FRACTAL customer account (`buyer@example.com` is seeded).

1. On the mock store, set the merchant handler to **Broken** (or use `/demo`).
2. Pay through PayPal Sandbox. The order stays `PAYMENT_PENDING`.
3. As the customer, open a case with the order reference and the claim code shown on the confirmation page. The store registers each code with FRACTAL at checkout, bound to the email entered on the storefront. Codes expire after 24 hours and are single-use.
   For local testing without a live PayPal checkout: `make mint-claim ORDER=ORDER-DEMO-1`.
4. As the merchant, review the evidence, the live Elastic/Kibana telemetry, the AI diagnosis and the PolicyGuard decision, then reconcile.
5. Try the negative cases from `/demo`: invalid-signature webhook, amount mismatch, refund without approval.
6. Fire 50 concurrent webhook replays. Exactly one action executes.
7. Open the audit timeline and run the hash-chain verification.

```bash
make demo   # scripted happy path plus negative scenarios
```

---

## AI Case Assistant

When one merchant has many customers reporting the same exception, FRACTAL groups
the cases into an **incident**, ranks what needs attention, and drafts the
customer replies. PolicyGuard and the merchant keep control of every action.

| Capability | What it does | Where |
|---|---|---|
| Incident grouping | Clusters cases that share verified facts (same provider account, same root cause, same failing webhook signal) inside a time window | `api/internal/assistant` |
| Triage inbox | Ranks incidents and cases by severity, amount, age and incident size, each with a one-line summary | `/merchant/incidents` |
| Drafted replies | Renders a reply per customer from verified case facts, for the merchant to edit, approve and send in bulk | `/merchant/incidents/{id}` |

### How grouping is verified

The assistant proposes the grouping; the Go backend checks it.

1. **Fingerprint** — a SHA-256 over merchant, provider account, root-cause code
   and failing webhook signal. Amounts and customer identity are excluded, so
   one incident can span many customers.
2. **Verified facts** — a case joins an incident only when the backend confirms
   the shared discrepancy from the database: the provider confirms the payment
   and the merchant order was never updated. A case that fails the check stays
   a standalone case in the inbox.
3. **Threshold** — `FRACTAL_INCIDENT_MIN_CASES` (default `2`) is how many cases
   must share a cause before it counts as a group. `FRACTAL_INCIDENT_WINDOW`
   (default `6h`) is how far apart two cases may be opened and still belong
   together.
4. **Corrections** — the merchant can add or remove a case from an incident.
   Removing a case only changes the grouping; it never changes that case's own
   policy path.

### How the priority is set

Priority is computed from verified facts only: severity from the risk engine,
payment amount, customer wait time and incident size. The AI writes the
summary; it never sets the priority.

### Drafted replies and the guardrails

- Drafts use a template filled with verified case data (order reference,
  captured amount, provider and order state). Nothing else.
- **No promises.** Drafts and merchant edits are scanned for promises about
  refunds, compensation or timelines and rejected with a `422`.
- **Money requests are never answered with a promise.** When a customer asks
  for money back, the draft is flagged `CUSTOMER_REQUESTED_MONEY` and routed to
  the merchant and PolicyGuard.
- **Customer messages are untrusted input.** Their content is classified, never
  executed and never copied into a draft.
- **AI-assisted labels.** Every sent message is stored with `ai_generated`,
  shows an "AI-assisted" badge to the customer, and is written to the audit
  timeline as `AI_MESSAGE_SENT`.
- **The assistant has no provider credentials** and cannot trigger actions.

### Fan-out keeps per-case policy checks

"Fix cause and reconcile all" proposes one action per linked case, and each
case goes through the same per-case path as a single-case action. PolicyGuard
evaluates every case on its own facts, so an amount mismatch blocks exactly one
case while the rest reconcile:

```text
INC-5073  WEBHOOK_ORDER_UPDATE_MISSING  5 cases
  FR-10005  ALLOW
  FR-10111  ALLOW
  FR-10112  ALLOW
  FR-10113  ALLOW
  FR-10127  BLOCK  (AMOUNT_MISMATCH, SEVERITY_CRITICAL)
```

### Configuration

| Variable | Default | Meaning |
|---|---|---|
| `FRACTAL_INCIDENT_MIN_CASES` | `2` | Cases that must share a cause before it becomes an incident |
| `FRACTAL_INCIDENT_WINDOW` | `6h` | Time window used for grouping |
| `FRACTAL_ASSISTANT_AUTOSEND` | `false` | Allow low-risk status updates to send without a merchant approval |

---

## Roles and visibility

| Item | Customer | Merchant staff | Merchant admin | Platform admin |
|---|---|---|---|---|
| Case, payment, order status | Yes | Yes | Yes | Read for audit |
| Provider order and capture IDs | No | Yes | Yes | Yes |
| AI findings | Simplified | Detailed | Detailed | Detailed |
| Evidence, Elastic logs and traces | No | Case-scoped | Case-scoped | Yes |
| Policy decision | Plain reason | Rules and findings | Rules and findings | Yes |
| Raw policy input, anomaly score | No | No | Yes | Yes |
| Audit timeline | Customer-visible | Customer and merchant-only | Customer and merchant-only | Full |
| Approve refund | No | No | Yes, not own request | No |
| Edit policy bundles | No | No | No | Yes |

A case ID is never a credential. Every request is checked for object-level access, and non-parties receive 404.

---

## Policy (PolicyGuard)

- Policies live in `policies/` as Rego modules, plus data files for thresholds, the action registry, merchant overrides and jurisdiction pack flags.
- Every rule module emits **findings**. A single aggregator applies fixed precedence: `BLOCK` over `WAIT` over `REQUIRE_APPROVAL` over `ALLOW`. `ALLOW` requires every gate listed for the action to pass.
- Go builds the input from facts it fetched itself. Agent output sits under `advisory` and no allow logic may read it (enforced in CI).
- Missing input, unknown action, unknown policy revision, or an OPA failure results in `BLOCK`. The system fails closed.
- Jurisdiction packs (for example US Regulation E, EU PSD2) are **disabled by default** and may only be enabled after qualified legal review.

```bash
make policy-test    # opa test, opa check --strict, opa fmt, advisory isolation, registry lint
```

> Note: `make policy-test` targets the Makefile, which is not part of this
> repository's tracked files. The exact Phase 11 commands below run directly.

---

## Anomaly detection (Phase 11)

An operational anomaly score that is **tighten-only**: it feeds
`risk.anomaly_score` into the OPA input and can escalate `ALLOW` to
`REQUIRE_APPROVAL` (`RSK-002`), but it can never turn `BLOCK` into `ALLOW`,
never authorizes an action by itself, and — with the model disabled or no
score available — the system behaves exactly as before (score is `null`).

### Components

| Piece | Location | Role |
|---|---|---|
| Feature extraction (5 features) | `investigator/app/anomaly/features.py`, `api/internal/anomaly/features.go` | per-merchant windows from existing telemetry, identical logic in both languages |
| Synthetic generator | `investigator/app/anomaly/synthetic.py` | labeled `SYNTHETIC` events; refuses to overwrite files |
| Model A — statistical thresholds | `investigator/app/anomaly/baseline.py` → `artifacts/baseline.json` | Tukey fences, transparent score |
| Model B — Isolation Forest | `investigator/app/anomaly/iforest.py` → `artifacts/iforest.json` | sklearn trees exported as JSON; Go walks them with no sklearn dependency |
| Model C — VAE + classifier | `investigator/app/anomaly/vae.py` | experimental; **not retained** (did not beat the best baseline by 0.05 AUC on the synthetic benchmark) |
| Evaluation/export | `investigator/app/anomaly/evaluate.py` | benchmark + artifact export + golden scores for parity tests |
| Go runtime scoring | `api/internal/anomaly/` | `go:embed` artifacts, golden parity tests, provenance |
| OPA input + fail-closed client | `api/internal/policy/`, `POST /internal/v1/policy/evaluate` | OPA down/500/malformed → `BLOCK` |
| Scoring CLI | `api/scripts/anomaly/` | writes `anomaly_scores` (migration `0008` adds read indexes) |
| Admin UI | `web/src/pages/merchant/CaseDetail.tsx` | anomaly card for merchant/platform admins only |

### Run and test (exact commands)

```bash
# 1. Policy: Phase 10 suite + Phase 11 tighten-only invariants (18 tests)
opa check --strict policies
opa test policies

# 2. Go: feature extraction, Python↔Go golden parity, fail-closed policy client
cd api
go test ./...
go vet -tags anomaly ./...          # includes the scoring CLI

# 3. Python: features, models, evaluation (46 tests)
cd investigator
python -m pytest tests -q

# 4. Re-train and export artifacts (SYNTHETIC benchmark — not real-world accuracy)
cd investigator
python -m app.anomaly.evaluate --export-dir ../api/internal/anomaly/artifacts

# 5. Score merchant telemetry into anomaly_scores (add -dry-run to preview only)
cd api
DATABASE_URL=postgresql://... go run -tags anomaly ./scripts/anomaly

# 6. API with the signal OFF (default) and ON
cd api
FRACTAL_ANOMALY_ENABLED=false go run ./cmd/fractal-api   # default: anomaly_score is null
FRACTAL_ANOMALY_ENABLED=true  OPA_URL=http://localhost:8181 go run ./cmd/fractal-api

# 7. Apply the additive migration
#    api/migrations/0008_anomaly_score_index.{up,down}.sql
```

### Live OPA demo: "this merchant's webhook failures are 8x normal"

There are two switches, and both must be on for the signal to act:

- **API side:** `FRACTAL_ANOMALY_ENABLED=true` (otherwise the Go handler sends
  `risk.anomaly_score: null` no matter what).
- **Policy side:** `anomaly_detection_enabled := true` in
  `policies/data.rego` (shipped default is `false`).

Start OPA (`opa run --server :8181 policies` or `make up`), then evaluate a
reconcile input with a high score (the webhook-failure burst scenario scores
~0.97):

```bash
curl -s localhost:8181/v1/data/fractal/guard/decision -H 'Content-Type: application/json' -d '{
  "input": {
    "schema_version": "1.0",
    "request": {"evaluated_at":"2026-11-10T10:00:00Z","action":{"type":"RECONCILE_MERCHANT_ORDER","params":{"amount_minor":4900,"currency":"USD"}}},
    "case": {"id":"case-1","case_number":"FR-10042","status":"INVESTIGATING"},
    "actor": {"id":"user-1","type":"MERCHANT_USER","merchant_id":"MER-501","permissions":["reconcile"]},
    "merchant": {"id":"MER-501","active":true},
    "facts": {
      "provider": {"name":"PAYPAL","status":"COMPLETED","amount_minor":4900,"currency":"USD","source":"PAYPAL_API","fetched_at":"2026-11-10T09:59:40Z"},
      "order": {"status":"PAYMENT_PENDING","amount_minor":4900,"currency":"USD","source":"MERCHANT_API","fetched_at":"2026-11-10T09:59:41Z"},
      "webhook": {"signature_verified":true,"event_count":1}
    },
    "execution": {"already_executed":false},
    "risk": {"severity":"MEDIUM","anomaly_score":0.97},
    "approvals": [], "applicability": []
  }
}' | python -m json.tool
```

Verified outcomes for that payload (score `0.97`):

| Policy bundle | Facts | Outcome |
|---|---|---|
| shipped (`enabled := false`) | as above | `ALLOW` — score ignored (disabled ≡ Phase 10) |
| `enabled := true` | as above | `REQUIRE_APPROVAL`, finding `RSK-002`, approver `MERCHANT_ADMIN` |
| `enabled := true` | `signature_verified: false` | `BLOCK` — a high score never rescues a bad signature |
| `enabled := true` | provider fetched an hour earlier | `WAIT` — evidence gates stay intact |
| `enabled := true` | `anomaly_score: null` | `ALLOW` — unscored ≡ Phase 10 |

### Guarantees (tested)

- **Disabled ≡ Phase 10:** `policies/tests/anomaly_test.rego` +
  `TestDisabledScorerReturnsNullScore` + `TestHandlerDisabledSendsNullScoreToOPA`.
- **High score → `REQUIRE_APPROVAL`, `MERCHANT_ADMIN`:** `RSK-002` tests.
- **`BLOCK` never becomes `ALLOW`:** invalid signature / amount mismatch with
  score `0.99` stay `BLOCK`.
- **Fail-closed:** OPA unreachable / HTTP 500 / malformed body / invalid
  outcome → `BLOCK` (`internal/policy` tests).
- **No cross-tenant mixing:** feature extraction rejects mismatched merchants
  (`ErrTenantMismatch` / `TenantMismatchError`); score lookups are always
  scoped to the case's merchant.
- **Admin-only UI:** the anomaly card/field appears only for
  `PLATFORM_ADMIN` or `MERCHANT_ADMIN` while enabled — never for customers or
  merchant staff.

### Data provenance

Training and evaluation use **synthetic** events
(`investigator/app/anomaly/synthetic.py`, `data_origin: "SYNTHETIC"`). All
metrics in `api/internal/anomaly/artifacts/evaluation_report.json` are
held-out synthetic metrics and must never be presented as real-world accuracy.
See the model card: [`investigator/app/anomaly/README.md`](investigator/app/anomaly/README.md).

---

## Development

### Repository layout

```text
fractal/
  api/                Go modular monolith (cases, auth, provider, webhook, evidence, risk, policy, executor, audit, assistant)
  investigator/       Python LangGraph service
  merchant-store/     Mock merchant with breakable webhook handler
  policies/           Rego modules, data, tests
  web/                React customer and merchant UI
  scripts/            seed, replay, load test
  docs/               Master document, spec, development plan
  docker-compose.yml
  Makefile
```

### Common commands

```bash
make up            # start all services
make down          # stop all services
make migrate-up    # run migrations
make seed          # seed dev data
make test          # Go, Python and web unit tests
make policy-test   # OPA tests and policy lint
make e2e           # end-to-end and negative scenario suite
make demo          # scripted demo flow
make reset         # wipe local data
```

### Testing

| Layer | What is checked |
|---|---|
| Policy | Positive and negative fixtures, advisory isolation, registry lint |
| Go | Authorization, projections, state machine, signature verification, idempotency |
| Integration | Dedup, advisory locks, append-only triggers, audit chain on a real PostgreSQL |

### Case assistant tests

`api/internal/assistant` has two test layers:

- **Pure tests** — fingerprinting, the verified-fact gate, priority ordering,
  the promise and money-request guardrails, and the draft templates.
- **Database tests** — grouping thresholds, merchant isolation of incidents and
  drafts, and the full draft lifecycle (draft → approve → send → audit entry).
  They create their own merchant and cases and clean up after themselves; they
  are skipped when `DATABASE_URL` is not set.

```bash
cd api && DATABASE_URL=postgres://fractal:fractal_dev_password@localhost:5433/fractal?sslmode=disable \
  go test ./internal/assistant/...
```
| Concurrency | 50 webhook replays and 20 parallel action requests produce one execution |
| Agent | Seeded scenarios, fabricated-evidence rejection, prompt-injection attempts |
| End to end | Happy path plus the negative scenario suite |

---

## Security notes

- PayPal Sandbox only. No live credentials.
- Webhooks are verified before storage. Invalid events go to `security_events` and never into `webhook_events`.
- Deduplication on `(provider, provider_event_id)` and idempotency keys on every action.
- The agent has read-only evidence tools and no PayPal credentials.
- Customer free text is treated as untrusted input to the agent.
- Secrets are stored by reference, never in the database or the repository.
- Evidence, policy decisions and audit events are append-only.
- FRACTAL stores identifiers, statuses and evidence references, not card data. Assess PCI scope before any production use.

---

## Documentation

| Document | Contents |
|---|---|
| [`docs/FRACTAL_Master_Document.md`](docs/FRACTAL_Master_Document.md) | Product definition, architecture, decisions, demo scenario |
| [`docs/FRACTAL_PolicyGuard_Schema_API_Spec.md`](docs/FRACTAL_PolicyGuard_Schema_API_Spec.md) | OPA design, PostgreSQL schema, API contracts |
| [`docs/FRACTAL_Development_Plan.md`](docs/FRACTAL_Development_Plan.md) | Phases, wireframes, compliance references, testing strategy |

External references: [PayPal Webhooks](https://developer.paypal.com/api/rest/webhooks/), [Open Policy Agent](https://www.openpolicyagent.org/docs/latest/), [OWASP API Security](https://owasp.org/www-project-api-security/), [PCI Security Standards Council](https://www.pcisecuritystandards.org/), [LangGraph](https://langchain-ai.github.io/langgraph/), [OpenTelemetry](https://opentelemetry.io/docs/).

---

## Roadmap

- **Core:** shared case, verified webhooks, evidence pipeline, AI investigation, PolicyGuard, idempotent execution, web UI.
- **Optional:** operational anomaly detection that can only tighten decisions *(shipped: Phase 11)*; proactive reconciler; root-cause grouping across cases.
- **Later:** additional providers behind the `Provider` interface, legally reviewed jurisdiction packs, production hardening.

## Known limitations

- Sandbox only; Sandbox behavior can differ from live PayPal.
- Anomaly models train on synthetic data plus your own Sandbox runs; exported metrics are synthetic-only and are labeled as such.
- No jurisdiction policy pack is enabled. FRACTAL makes no regulatory compliance claims.
- The mock merchant stands in for a real merchant integration. Real merchants must implement the merchant integration contract in the spec.

## Contributing

Issues and pull requests are welcome. Run `make test`, `make policy-test` and `make e2e` before opening a PR. New actions must be registered in `policies/data/actions.json` with required gates and tests.

## License

*FRACTAL is an engineering and product project. It is not legal advice, a compliance certification, or a determination that any regulation applies to a particular merchant, provider or transaction. Regulatory policy packs must be validated by qualified legal and compliance professionals before production use.*