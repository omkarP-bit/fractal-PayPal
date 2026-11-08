# FRACTAL: PolicyGuard, Database Schema and API Contracts

Version 1.0, 6 November 2026. Companion to the FRACTAL Project Master Document. Part A: standard and flexible OPA design. Part B: PostgreSQL schema. Part C: API contracts.

---

# Part A: Standard and flexible OPA design

## A1. Design goals

- **Standard:** one input envelope, one decision output, one aggregation rule. Every rule module looks the same.
- **Flexible:** behavior lives in data (thresholds, merchant overrides, action registry, jurisdiction packs), not in code. New actions and merchants need data changes, not new Rego.
- **Safe:** deny precedence is fixed (BLOCK, then WAIT, then REQUIRE\_APPROVAL, then ALLOW). ALLOW needs every required gate to pass explicitly. Any error or missing fact fails closed.
- **Testable:** every rule has positive and negative fixtures.
- **Auditable:** every decision carries policy revision, rule IDs, reason codes and an input hash.

## A2. The rule that makes OPA trustworthy

OPA evaluates whatever input it receives. It cannot know if a claim is true. So the contract is:

1. **Go builds the input** from sources it fetched itself (PayPal API, merchant API, its own database).
2. Every fact carries `source` and `fetched_at`.
3. Agent output lives only under `advisory`. No ALLOW logic may read `advisory`. A CI check enforces this (A10).
4. If the input is invalid or incomplete, the decision is BLOCK or WAIT, never ALLOW.

## A3. Standard input envelope (schema\_version 1.0)

Go validates this against JSON Schema before calling OPA. Money is always integer minor units plus an ISO currency code. Timestamps are RFC 3339 UTC.

```json
{
  "schema_version": "1.0",
  "request": {
    "evaluation_id": "uuid",
    "evaluated_at": "2026-11-06T10:00:00Z",
    "action": {
      "type": "RECONCILE_MERCHANT_ORDER",
      "params": { "amount_minor": 4900, "currency": "USD" },
      "params_sha256": "hex",
      "idempotency_key": "string"
    }
  },
  "case": { "id": "uuid", "case_number": "FR-10042", "status": "INVESTIGATING" },
  "actor": { "id": "uuid", "type": "MERCHANT_USER", "merchant_id": "uuid", "permissions": ["reconcile"] },
  "merchant": { "id": "uuid", "active": true },
  "facts": {
    "provider": { "name": "PAYPAL", "status": "COMPLETED", "amount_minor": 4900, "currency": "USD",
                  "source": "PAYPAL_API", "fetched_at": "2026-11-06T09:59:40Z" },
    "order":    { "status": "PAYMENT_PENDING", "amount_minor": 4900, "currency": "USD",
                  "source": "MERCHANT_API", "fetched_at": "2026-11-06T09:59:41Z" },
    "webhook":  { "signature_verified": true, "event_count": 1 }
  },
  "execution": { "already_executed": false },
  "risk": { "severity": "MEDIUM", "anomaly_score": null },
  "approvals": [],
  "applicability": [ { "pack_id": "paypal_integration", "version": "1.0", "status": "APPLICABLE" } ],
  "advisory": { "agent": { "recommendation": "RECONCILE_MERCHANT_ORDER", "confidence": 0.97 } }
}
```

## A4. Standard decision output

```json
{
  "outcome": "ALLOW",
  "action": "RECONCILE_MERCHANT_ORDER",
  "findings": [
    { "rule_id": "PP-101", "outcome": "ALLOW", "gate": "provider_completed", "code": "GATE_PASSED", "message": "Provider reports COMPLETED" }
  ],
  "missing_gates": [],
  "required_approvals": [],
  "policy": { "revision": "2026-11-06.1", "schema_version": "1.0" }
}
```

Go stores the decision, the full input and its SHA-256 in `policy_decisions`. A missing, malformed or timed-out decision is treated as BLOCK.

## A5. Layout

```text
policies/
  lib/lib.rego                      helpers: finding(), gate(), evidence_age_seconds()
  guard/guard.rego                  aggregator: precedence, required gates, decision
  rules/
    integrity.rego                  schema version, required fields, policy revision known
    security.rego                   webhook signature, idempotency, permission, tenant match
    evidence.rego                   freshness and presence of facts
    paypal.rego                     provider status, amount and currency match
    approvals.rego                  approval requirements and valid approvals
    risk.rego                       severity and anomaly (tighten only)
    pack_us_reg_e.rego              jurisdiction pack, disabled until legally reviewed
  data/
    config.json                     thresholds, supported versions
    actions.json                    action registry (gates, permissions, approval)
    merchants.json                  per-merchant overrides
    packs.json                      pack enable flags and legal review status
    meta.json                       bundle revision (written by the build)
  tests/                            fixtures and *_test.rego
```

Rule modules live at `data.fractal.rules.<name>` and each exposes a `findings` set. The aggregator discovers them automatically, so adding a module needs no change to the guard.

## A6. Aggregator (precedence and required gates)

```rego
package fractal.guard

import rego.v1
import data.fractal.lib

# 1. Collect findings from every rule module
module_findings contains f if {
    some pkg
    f := data.fractal.rules[pkg].findings[_]
}

# 2. Gates that explicitly passed
gates_passed contains f.gate if {
    some f in module_findings
    f.outcome == "ALLOW"
    f.gate
}

# 3. Required gates come from the action registry (data, not code)
required_gates := object.get(data.fractal.actions, [input.request.action.type, "required_gates"], [])

missing_gates := {g | some g in required_gates; not g in gates_passed}

gate_findings contains lib.finding("GRD-001", "BLOCK", "REQUIRED_GATE_MISSING",
    sprintf("Required gate not satisfied: %s", [g])) if {
    some g in missing_gates
}

gate_findings contains lib.finding("GRD-002", "BLOCK", "NO_GATES_DEFINED",
    "Action defines no required gates") if {
    count(required_gates) == 0
}

all_findings := module_findings | gate_findings

has(o) if {
    some f in all_findings
    f.outcome == o
}

# 4. Fixed precedence: BLOCK > WAIT > REQUIRE_APPROVAL > ALLOW. Default is BLOCK.
default outcome := "BLOCK"

outcome := "BLOCK" if has("BLOCK")

outcome := "WAIT" if {
    not has("BLOCK")
    has("WAIT")
}

outcome := "REQUIRE_APPROVAL" if {
    not has("BLOCK")
    not has("WAIT")
    has("REQUIRE_APPROVAL")
}

outcome := "ALLOW" if {
    not has("BLOCK")
    not has("WAIT")
    not has("REQUIRE_APPROVAL")
    has("ALLOW")
}

decision := {
    "outcome": outcome,
    "action": object.get(input, ["request", "action", "type"], null),
    "findings": [f | some f in all_findings],
    "missing_gates": [g | some g in missing_gates],
    "required_approvals": [f.approver_role | some f in all_findings; f.code == "APPROVAL_REQUIRED"],
    "policy": {
        "revision": object.get(data.fractal, ["meta", "revision"], "UNKNOWN"),
        "schema_version": object.get(input, "schema_version", "UNKNOWN"),
    },
}
```

Why this is safe: an ALLOW needs every registry gate to pass and no finding of a stronger outcome. An unrelated allow rule can never override a BLOCK, WAIT or REQUIRE\_APPROVAL.

## A7. Shared library and rule modules

```rego
package fractal.lib

import rego.v1

finding(rule_id, outcome, code, message) := {
    "rule_id": rule_id, "outcome": outcome, "code": code, "message": message,
}

gate(name, rule_id, message) := {
    "rule_id": rule_id, "outcome": "ALLOW", "gate": name, "code": "GATE_PASSED", "message": message,
}

evidence_age_seconds(fetched_at) := age if {
    now_ns := time.parse_rfc3339_ns(input.request.evaluated_at)
    fetched_ns := time.parse_rfc3339_ns(fetched_at)
    age := (now_ns - fetched_ns) / 1000000000
}
```

**integrity.rego**

```rego
package fractal.rules.integrity

import rego.v1
import data.fractal.lib

findings contains lib.finding("INT-001", "BLOCK", "INPUT_SCHEMA_UNSUPPORTED", "Unsupported input schema version") if {
    not input.schema_version in data.fractal.config.supported_input_versions
}

findings contains lib.finding("INT-002", "BLOCK", "ACTION_UNKNOWN", "Action is not in the registry") if {
    not data.fractal.actions[input.request.action.type]
}

findings contains lib.finding("INT-003", "BLOCK", "INPUT_INCOMPLETE", sprintf("Missing input section: %s", [k])) if {
    some k in ["request", "case", "actor", "merchant", "facts", "execution", "risk"]
    not input[k]
}

findings contains lib.finding("INT-004", "BLOCK", "POLICY_VERSION_UNKNOWN", "Policy revision is not set") if {
    object.get(data.fractal, ["meta", "revision"], "UNKNOWN") == "UNKNOWN"
}
```

**security.rego**

```rego
package fractal.rules.security

import rego.v1
import data.fractal.lib

required_permission := data.fractal.actions[input.request.action.type].required_permission

findings contains lib.finding("SEC-001", "BLOCK", "WEBHOOK_SIGNATURE_INVALID", "Webhook signature not verified") if {
    input.facts.webhook.signature_verified == false
}

findings contains lib.finding("SEC-002", "BLOCK", "ACTION_ALREADY_EXECUTED", "Action already executed for this key") if {
    input.execution.already_executed == true
}

findings contains lib.finding("SEC-003", "BLOCK", "PERMISSION_DENIED", "Actor lacks the required permission") if {
    not required_permission in input.actor.permissions
}

findings contains lib.finding("SEC-004", "BLOCK", "TENANT_MISMATCH", "Actor does not belong to this merchant") if {
    input.actor.merchant_id != input.merchant.id
}

findings contains lib.gate("webhook_verified", "SEC-101", "Webhook signature verified") if {
    input.facts.webhook.signature_verified == true
}

findings contains lib.gate("not_executed", "SEC-102", "Action not yet executed") if {
    input.execution.already_executed == false
}

findings contains lib.gate("actor_permitted", "SEC-103", "Actor permission and tenant verified") if {
    required_permission in input.actor.permissions
    input.actor.merchant_id == input.merchant.id
    input.merchant.active == true
}
```

**evidence.rego**

```rego
package fractal.rules.evidence

import rego.v1
import data.fractal.lib

max_age := data.fractal.config.max_evidence_age_seconds

findings contains lib.finding("EVD-001", "WAIT", "EVIDENCE_STALE", "Provider state is older than allowed") if {
    lib.evidence_age_seconds(input.facts.provider.fetched_at) > max_age
}

findings contains lib.finding("EVD-002", "WAIT", "EVIDENCE_MISSING", "Provider facts are missing") if {
    not input.facts.provider.fetched_at
}

findings contains lib.gate("evidence_fresh", "EVD-101", "Provider state is fresh") if {
    lib.evidence_age_seconds(input.facts.provider.fetched_at) <= max_age
}
```

**paypal.rego**

```rego
package fractal.rules.paypal

import rego.v1
import data.fractal.lib

is_paypal if input.facts.provider.name == "PAYPAL"

findings contains lib.finding("PP-001", "WAIT", "PROVIDER_NOT_COMPLETED", "Provider has not reported COMPLETED") if {
    is_paypal
    input.facts.provider.status != "COMPLETED"
}

findings contains lib.finding("PP-002", "BLOCK", "AMOUNT_MISMATCH", "Provider and order amounts differ") if {
    is_paypal
    input.facts.provider.amount_minor != input.facts.order.amount_minor
}

findings contains lib.finding("PP-003", "BLOCK", "CURRENCY_MISMATCH", "Provider and order currencies differ") if {
    is_paypal
    input.facts.provider.currency != input.facts.order.currency
}

findings contains lib.gate("provider_completed", "PP-101", "Provider reports COMPLETED") if {
    is_paypal
    input.facts.provider.status == "COMPLETED"
}

findings contains lib.gate("amounts_match", "PP-102", "Amount and currency match") if {
    is_paypal
    input.facts.provider.amount_minor == input.facts.order.amount_minor
    input.facts.provider.currency == input.facts.order.currency
}
```

**approvals.rego** (approval is bound to the exact action parameters)

```rego
package fractal.rules.approvals

import rego.v1
import data.fractal.lib

action_cfg := data.fractal.actions[input.request.action.type]
merchant_cfg := object.get(data.fractal.merchants, input.merchant.id, {})

needs_approval if action_cfg.requires_approval == true

needs_approval if {
    input.request.action.params.amount_minor > merchant_cfg.approval_threshold_minor
}

valid_approval if {
    some a in input.approvals
    a.decision == "APPROVED"
    a.action == input.request.action.type
    a.params_sha256 == input.request.action.params_sha256
    not a.revoked
    time.parse_rfc3339_ns(a.expires_at) > time.parse_rfc3339_ns(input.request.evaluated_at)
    a.approver_id != input.actor.id
}

findings contains {
    "rule_id": "APR-001", "outcome": "REQUIRE_APPROVAL", "code": "APPROVAL_REQUIRED",
    "message": "A valid approval is required", "approver_role": role,
} if {
    needs_approval
    not valid_approval
    role := object.get(action_cfg, "approver_role", "MERCHANT_ADMIN")
}

findings contains lib.gate("approval_satisfied", "APR-101", "Valid approval present") if {
    needs_approval
    valid_approval
}

findings contains lib.gate("approval_satisfied", "APR-102", "No approval required") if {
    not needs_approval
}
```

**risk.rego** (anomaly can only tighten)

```rego
package fractal.rules.risk

import rego.v1
import data.fractal.lib

findings contains lib.finding("RSK-001", "BLOCK", "SEVERITY_CRITICAL", "Critical operational risk") if {
    input.risk.severity == "CRITICAL"
}

findings contains {
    "rule_id": "RSK-002", "outcome": "REQUIRE_APPROVAL", "code": "APPROVAL_REQUIRED",
    "message": "Anomaly score above threshold", "approver_role": "MERCHANT_ADMIN",
} if {
    is_number(input.risk.anomaly_score)
    input.risk.anomaly_score >= data.fractal.config.anomaly_approval_threshold
}

findings contains lib.gate("risk_acceptable", "RSK-101", "Risk severity acceptable") if {
    input.risk.severity in {"LOW", "MEDIUM", "HIGH"}
}
```

Note that HIGH severity still passes the gate. HIGH-severity scenarios such as amount mismatch are blocked by their own specific rules, not by a blanket severity rule.

## A8. Policy data (where the flexibility lives)

```json
{
  "fractal": {
    "meta": { "revision": "2026-11-06.1" },
    "config": {
      "supported_input_versions": ["1.0"],
      "max_evidence_age_seconds": 120,
      "anomaly_approval_threshold": 0.8
    },
    "actions": {
      "RECONCILE_MERCHANT_ORDER": {
        "required_permission": "reconcile",
        "requires_approval": false,
        "required_gates": ["webhook_verified", "not_executed", "actor_permitted", "evidence_fresh",
                           "provider_completed", "amounts_match", "approval_satisfied", "risk_acceptable"]
      },
      "REFUND_PAYMENT": {
        "required_permission": "refund",
        "requires_approval": true,
        "approver_role": "MERCHANT_ADMIN",
        "required_gates": ["not_executed", "actor_permitted", "evidence_fresh",
                           "provider_completed", "approval_satisfied", "risk_acceptable"]
      }
    },
    "merchants": {
      "MER-501": { "approval_threshold_minor": 20000 }
    },
    "packs": {
      "us_reg_e": { "enabled": false, "legal_review": "PENDING" }
    }
  }
}
```

Adding a new action means one registry entry listing its gates. Changing a threshold or a merchant limit is a data change with a new bundle revision, not a code change.

## A9. Jurisdiction packs (post-MVP)

- A pack is one more module at `fractal.rules.pack_<id>` with the same `findings` contract.
- It runs only when all three hold: `data.fractal.packs[id].enabled == true`, `legal_review == "APPROVED"` and the case applicability entry for that pack has `status == "APPLICABLE"`.
- If the registry lists a pack for an action and its applicability is `REQUIRES_REVIEW`, the module emits a WAIT finding (`APPLICABILITY_UNRESOLVED`). It never invents an obligation.
- Packs are disabled by default. Never claim compliance from the existence of a pack.

## A10. Testing, CI and deployment

```rego
package fractal.guard_test

import rego.v1
import data.fractal.guard
import data.fractal.fixtures

test_reconcile_allowed if {
    d := guard.decision with input as fixtures.reconcile_ok
    d.outcome == "ALLOW"
}

test_invalid_signature_blocks if {
    inp := json.patch(fixtures.reconcile_ok, [{"op": "replace", "path": "/facts/webhook/signature_verified", "value": false}])
    d := guard.decision with input as inp
    d.outcome == "BLOCK"
}

test_amount_mismatch_blocks if {
    inp := json.patch(fixtures.reconcile_ok, [{"op": "replace", "path": "/facts/order/amount_minor", "value": 5000}])
    d := guard.decision with input as inp
    d.outcome == "BLOCK"
}

test_stale_evidence_waits if {
    inp := json.patch(fixtures.reconcile_ok, [{"op": "replace", "path": "/facts/provider/fetched_at", "value": "2026-11-06T09:00:00Z"}])
    d := guard.decision with input as inp
    d.outcome == "WAIT"
}

test_refund_requires_approval if {
    d := guard.decision with input as fixtures.refund_no_approval
    d.outcome == "REQUIRE_APPROVAL"
}

test_unknown_action_blocks if {
    inp := json.patch(fixtures.reconcile_ok, [{"op": "replace", "path": "/request/action/type", "value": "DELETE_EVERYTHING"}])
    d := guard.decision with input as inp
    d.outcome == "BLOCK"
}

test_agent_advice_alone_cannot_allow if {
    d := guard.decision with input as fixtures.agent_only
    d.outcome != "ALLOW"
}
```

**CI checks**

- `opa test ./policies -v` with a coverage threshold.
- `opa check --strict ./policies` and `opa fmt --diff`.
- Advisory isolation: `grep -rn "input.advisory" policies/rules | grep -v "rules/risk"` must return nothing.
- Registry lint: every action lists at least one gate, and every listed gate is emitted by some rule.
- Build step writes `meta.json` revision, then `opa build` produces a bundle. Keep old bundles for rollback.

**Runtime**

- Hackathon: OPA as a sidecar, `opa run --server --bundle ./bundle`. Go calls `POST /v1/data/fractal/guard/decision` with `{"input": ...}`.
- Timeout, non-200 response or missing `result` means BLOCK, recorded as an evaluation failure.
- Enable OPA decision logs and ship them to Elastic.

## A11. Recipe: add a new rule, action or provider

1. New action: add a registry entry (permission, approval flag, required gates).
2. New rule: create `rules/<name>.rego` with a `findings` set using `lib.finding` and `lib.gate`.
3. New provider: add `rules/<provider>.rego` guarded by `facts.provider.name`, emitting the same gate names (`provider_completed`, `amounts_match`).
4. Add positive and negative fixtures, run CI, bump the revision.

---

# Part B: PostgreSQL schema

Target PostgreSQL 15 or later. Conventions: UUID primary keys, `timestamptz` everywhere, money as `bigint` minor units plus `char(3)` currency, text columns with CHECK constraints instead of enum types (easier to evolve). Secrets are stored as references, never as values.

## B1. Identity and tenancy

```sql
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email citext NOT NULL UNIQUE,
  display_name text NOT NULL,
  kind text NOT NULL CHECK (kind IN ('CUSTOMER','MERCHANT_USER','PLATFORM_ADMIN','SERVICE')),
  status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','SUSPENDED')),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE merchants (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  legal_name text NOT NULL,
  country char(2) NOT NULL,
  status text NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','SUSPENDED')),
  integration_base_url text NOT NULL,
  integration_key_ref text NOT NULL,
  policy_config jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE merchant_members (
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  user_id uuid NOT NULL REFERENCES users(id),
  role text NOT NULL CHECK (role IN ('MERCHANT_ADMIN','MERCHANT_STAFF')),
  permissions text[] NOT NULL DEFAULT '{}',
  PRIMARY KEY (merchant_id, user_id)
);

CREATE TABLE provider_accounts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  provider text NOT NULL CHECK (provider IN ('PAYPAL')),
  environment text NOT NULL CHECK (environment IN ('SANDBOX','LIVE')),
  external_account_id text NOT NULL,
  webhook_id text,
  credentials_ref text NOT NULL,
  status text NOT NULL DEFAULT 'ACTIVE',
  UNIQUE (provider, environment, external_account_id)
);
```

## B2. Payments and orders

```sql
CREATE TABLE payment_transactions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  provider_account_id uuid NOT NULL REFERENCES provider_accounts(id),
  provider text NOT NULL,
  provider_order_id text NOT NULL,
  capture_id text,
  amount_minor bigint NOT NULL CHECK (amount_minor >= 0),
  currency char(3) NOT NULL,
  provider_status text NOT NULL,
  merchant_order_ref text,
  last_fetched_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (provider, provider_order_id)
);
CREATE UNIQUE INDEX uq_payment_capture ON payment_transactions (provider, capture_id) WHERE capture_id IS NOT NULL;

-- Point-in-time copies of the merchant order (the merchant system stays the source of truth)
CREATE TABLE order_snapshots (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  merchant_order_ref text NOT NULL,
  amount_minor bigint NOT NULL CHECK (amount_minor >= 0),
  currency char(3) NOT NULL,
  order_status text NOT NULL,
  fetched_at timestamptz NOT NULL,
  payload_sha256 text NOT NULL
);
CREATE INDEX ix_order_snapshots_latest ON order_snapshots (merchant_id, merchant_order_ref, fetched_at DESC);

-- Proof of ownership for case creation (token shown on the merchant confirmation page)
CREATE TABLE order_claim_tokens (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  merchant_order_ref text NOT NULL,
  customer_email citext NOT NULL,
  token_hash text NOT NULL,
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz,
  UNIQUE (merchant_id, merchant_order_ref, token_hash)
);
```

## B3. Shared case

```sql
CREATE SEQUENCE case_number_seq START 10000;

CREATE TABLE exception_cases (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_number text NOT NULL UNIQUE DEFAULT ('FR-' || nextval('case_number_seq')),
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  customer_id uuid NOT NULL REFERENCES users(id),
  payment_transaction_id uuid REFERENCES payment_transactions(id),
  merchant_order_ref text NOT NULL,
  case_type text NOT NULL CHECK (case_type IN
    ('PROVIDER_MERCHANT_STATE_MISMATCH','AMOUNT_MISMATCH','DUPLICATE_CHARGE','OTHER')),
  status text NOT NULL DEFAULT 'OPEN' CHECK (status IN
    ('OPEN','COLLECTING_EVIDENCE','INVESTIGATING','AWAITING_MERCHANT_APPROVAL',
     'AWAITING_CUSTOMER_EVIDENCE','ACTION_PENDING','RESOLVED','ESCALATED','REJECTED','CLOSED')),
  opened_via text NOT NULL CHECK (opened_via IN ('CUSTOMER','MERCHANT','AUTO_RECONCILER')),
  resolution_code text,
  version int NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  closed_at timestamptz
);
-- One live shared case per transaction and type
CREATE UNIQUE INDEX uq_live_case ON exception_cases (payment_transaction_id, case_type)
  WHERE status NOT IN ('RESOLVED','REJECTED','CLOSED');
CREATE INDEX ix_cases_merchant ON exception_cases (merchant_id, status);
CREATE INDEX ix_cases_customer ON exception_cases (customer_id, status);

CREATE TABLE case_messages (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id uuid NOT NULL REFERENCES exception_cases(id),
  author_id uuid REFERENCES users(id),
  author_type text NOT NULL CHECK (author_type IN ('CUSTOMER','MERCHANT','SYSTEM','AGENT')),
  visibility text NOT NULL DEFAULT 'SHARED' CHECK (visibility IN ('SHARED','MERCHANT_ONLY','INTERNAL')),
  body text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_messages_case ON case_messages (case_id, created_at);
```

## B4. Webhooks, security events and evidence

```sql
-- Only verified events are ever stored here. Rejected ones go to security_events.
CREATE TABLE webhook_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  provider text NOT NULL,
  provider_event_id text NOT NULL,
  provider_account_id uuid REFERENCES provider_accounts(id),
  event_type text NOT NULL,
  resource_id text,
  signature_verified boolean NOT NULL CHECK (signature_verified),
  payload jsonb NOT NULL,
  payload_sha256 text NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now(),
  processing_status text NOT NULL DEFAULT 'RECEIVED'
    CHECK (processing_status IN ('RECEIVED','PROCESSED','FAILED')),
  processed_at timestamptz,
  UNIQUE (provider, provider_event_id)          -- deduplication
);
CREATE INDEX ix_webhook_resource ON webhook_events (provider, resource_id);

CREATE TABLE security_events (
  id bigserial PRIMARY KEY,
  event_type text NOT NULL,                      -- e.g. WEBHOOK_SIGNATURE_INVALID, CASE_ACCESS_DENIED
  provider text,
  source_ip inet,
  payload_sha256 text,
  detail jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE case_evidence (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id uuid NOT NULL REFERENCES exception_cases(id),
  evidence_type text NOT NULL CHECK (evidence_type IN
    ('PROVIDER_TRANSACTION','ORDER_SNAPSHOT','WEBHOOK_EVENT','TELEMETRY_LOG','TELEMETRY_TRACE','CUSTOMER_UPLOAD')),
  source_system text NOT NULL CHECK (source_system IN ('PAYPAL_API','MERCHANT_API','FRACTAL_DB','ELASTIC','CUSTOMER')),
  source_ref text NOT NULL,
  fetched_at timestamptz NOT NULL,
  payload jsonb NOT NULL,
  payload_sha256 text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_evidence_case ON case_evidence (case_id, fetched_at);
```

## B5. Investigation, policy, approvals, execution

```sql
CREATE TABLE investigations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id uuid NOT NULL REFERENCES exception_cases(id),
  run_number int NOT NULL,
  agent_version text NOT NULL,
  model_id text NOT NULL,
  findings jsonb NOT NULL,
  recommendation text,
  suggested_severity text CHECK (suggested_severity IN ('LOW','MEDIUM','HIGH','CRITICAL')),
  evidence_ids uuid[] NOT NULL DEFAULT '{}',
  validation_status text NOT NULL CHECK (validation_status IN ('SCHEMA_VALID','REFS_VERIFIED','REJECTED')),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (case_id, run_number)
);

CREATE TABLE policy_bundles (
  revision text PRIMARY KEY,
  bundle_sha256 text NOT NULL,
  status text NOT NULL CHECK (status IN ('STAGED','ACTIVE','RETIRED')),
  activated_at timestamptz,
  activated_by uuid REFERENCES users(id)
);

CREATE TABLE policy_decisions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id uuid NOT NULL REFERENCES exception_cases(id),
  evaluation_id uuid NOT NULL UNIQUE,
  action_type text NOT NULL,
  params_sha256 text NOT NULL,
  input jsonb NOT NULL,
  input_sha256 text NOT NULL,
  outcome text NOT NULL CHECK (outcome IN ('ALLOW','REQUIRE_APPROVAL','WAIT','BLOCK')),
  findings jsonb NOT NULL,
  policy_revision text NOT NULL,
  evaluation_error text,
  requested_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_decisions_case ON policy_decisions (case_id, created_at);

CREATE TABLE approvals (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id uuid NOT NULL REFERENCES exception_cases(id),
  policy_decision_id uuid NOT NULL REFERENCES policy_decisions(id),
  action_type text NOT NULL,
  params_sha256 text NOT NULL,                  -- approval is bound to exact parameters
  requested_role text NOT NULL,
  approver_id uuid REFERENCES users(id),
  decision text NOT NULL DEFAULT 'PENDING' CHECK (decision IN ('PENDING','APPROVED','REJECTED')),
  reason text,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  decided_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE action_executions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  case_id uuid NOT NULL REFERENCES exception_cases(id),
  policy_decision_id uuid NOT NULL REFERENCES policy_decisions(id),
  action_type text NOT NULL,
  params jsonb NOT NULL,
  params_sha256 text NOT NULL,
  idempotency_key text NOT NULL UNIQUE,
  status text NOT NULL DEFAULT 'IN_PROGRESS' CHECK (status IN ('IN_PROGRESS','SUCCEEDED','FAILED')),
  response jsonb,
  error text,
  started_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);
-- At most one live or successful execution per case, action and parameters
CREATE UNIQUE INDEX uq_action_once ON action_executions (case_id, action_type, params_sha256)
  WHERE status IN ('IN_PROGRESS','SUCCEEDED');
```

## B6. Audit, outbox and optional anomaly scores

```sql
CREATE TABLE audit_events (
  seq bigserial PRIMARY KEY,
  case_id uuid REFERENCES exception_cases(id),
  actor_type text NOT NULL CHECK (actor_type IN ('CUSTOMER','MERCHANT_USER','SYSTEM','AGENT','POLICY','PROVIDER')),
  actor_id text,
  event_type text NOT NULL,
  visibility text NOT NULL CHECK (visibility IN ('CUSTOMER','MERCHANT','INTERNAL')),
  data jsonb NOT NULL DEFAULT '{}',
  prev_hash text,                               -- previous event_hash for the same case
  event_hash text NOT NULL,                     -- sha256(prev_hash || canonical event)
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ix_audit_case ON audit_events (case_id, seq);

-- Transactional outbox for notifications and portal updates
CREATE TABLE outbox_events (
  id bigserial PRIMARY KEY,
  case_id uuid REFERENCES exception_cases(id),
  topic text NOT NULL,
  payload jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  published_at timestamptz
);

-- Phase 2: operational anomaly layer
CREATE TABLE anomaly_scores (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id uuid NOT NULL REFERENCES merchants(id),
  case_id uuid REFERENCES exception_cases(id),
  model_id text NOT NULL,
  score double precision NOT NULL,
  features jsonb NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
```

## B7. Integrity guards

```sql
-- Evidence, decisions and audit rows are append-only
CREATE FUNCTION forbid_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION '% on % is not allowed', TG_OP, TG_TABLE_NAME;
END $$;

CREATE TRIGGER trg_evidence_immutable BEFORE UPDATE OR DELETE ON case_evidence
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER trg_decisions_immutable BEFORE UPDATE OR DELETE ON policy_decisions
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER trg_audit_immutable BEFORE UPDATE OR DELETE ON audit_events
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- Optional row-level security for object-level authorization (defense in depth)
ALTER TABLE exception_cases ENABLE ROW LEVEL SECURITY;
CREATE POLICY case_party_access ON exception_cases USING (
  customer_id = current_setting('app.user_id')::uuid
  OR merchant_id IN (SELECT merchant_id FROM merchant_members WHERE user_id = current_setting('app.user_id')::uuid)
);
```

## B8. Execution pattern (concurrency and idempotency)

```sql
BEGIN;
  SELECT pg_advisory_xact_lock(hashtextextended(:case_id::text, 0));   -- per-case lock
  INSERT INTO action_executions (case_id, policy_decision_id, action_type, params, params_sha256, idempotency_key)
  VALUES (:case_id, :decision_id, :action, :params, :params_hash, :idem_key)
  ON CONFLICT DO NOTHING
  RETURNING id;
  -- No row returned: duplicate. Load the stored result and return it. Do not execute again.
COMMIT;
-- 1. Call PayPal for a fresh state check, then the merchant API with the same Idempotency-Key.
-- 2. In a second transaction, set status SUCCEEDED or FAILED, append the audit event, write the outbox row.
-- 3. A crash leaves IN_PROGRESS. A recovery job retries with the same key (merchant endpoint is idempotent).
```

Webhook ingestion uses `INSERT ... ON CONFLICT (provider, provider_event_id) DO NOTHING RETURNING id`. No row returned means a replay: respond 200 and do nothing.

---

# Part C: API contracts

## C1. Conventions

- Base path `/api/v1`. JSON only. TLS only. Times are RFC 3339 UTC. Money is integer minor units plus ISO currency.
- **Auth:** `Authorization: Bearer <JWT>` with claims `sub`, `kind` (CUSTOMER, MERCHANT\_USER, PLATFORM\_ADMIN, SERVICE), `merchant_id`, `role`, `permissions[]`.
- **Object-level authorization on every request.** A case ID is never a credential. If the caller is not a party to the object, return 404, not 403, so IDs cannot be enumerated. Log a `security_events` row.
- **Idempotency:** every mutating POST requires `Idempotency-Key`. Same key and same body returns the stored response. Same key and different body returns 409.
- **Optimistic concurrency:** case reads return `ETag` (the case `version`). Case-changing requests may send `If-Match`. Mismatch returns 412.
- **Pagination:** `?limit=&cursor=`; responses carry `next_cursor`.
- **Tracing:** every response carries `X-Request-Id`, also written to logs and traces in Elastic.
- **Errors:** RFC 9457 `application/problem+json`.

```json
{
  "type": "https://fractal.dev/problems/validation-failed",
  "title": "Validation failed",
  "status": 400,
  "detail": "params.amount_minor must be a positive integer",
  "request_id": "req_8f2c",
  "errors": [ { "field": "params.amount_minor", "code": "INVALID" } ]
}
```

| Status | Problem type | When |
| --- | --- | --- |
| 400 | validation-failed | Malformed or invalid body |
| 401 | unauthenticated | Missing or invalid token |
| 403 | forbidden | Authenticated, in scope, but role lacks permission |
| 404 | not-found | Missing object or caller not a party |
| 409 | idempotency-conflict, invalid-state | Key reused with different body; transition not allowed |
| 412 | version-mismatch | `If-Match` does not match case version |
| 422 | evidence-rejected | Submission failed validation (for example unknown evidence reference) |
| 429 | rate-limited | Too many requests |
| 503 | dependency-unavailable | PayPal, OPA or merchant API unreachable. Fails closed, nothing executed |

Policy decisions are results, not errors. A BLOCK returns 200 with the decision in the body.

## C2. Role-scoped projections

The same case, two views. The server builds the projection from the caller’s role. Clients never filter.

**Customer view** (`GET /cases/{id}` as CUSTOMER)

```json
{
  "id": "uuid", "case_number": "FR-10042", "status": "INVESTIGATING",
  "merchant": { "id": "uuid", "name": "Acme Store" },
  "payment": { "status": "COMPLETED", "amount_minor": 4900, "currency": "USD" },
  "order": { "ref": "ORDER-789", "status": "PAYMENT_PENDING" },
  "summary": "Your payment was completed but the merchant order has not updated yet. The merchant has been notified.",
  "next_step": "No action needed. Please do not pay again.",
  "version": 4, "updated_at": "2026-11-06T10:00:00Z"
}
```

**Merchant view** (same path as MERCHANT\_USER) adds:

```json
{
  "provider": { "name": "PAYPAL", "order_id": "PAYPAL-ORDER-123", "capture_id": "CAPTURE-456", "fetched_at": "..." },
  "investigation": { "category": "PROVIDER_MERCHANT_STATE_MISMATCH", "root_cause_hypothesis": "...",
                     "evidence_ids": ["uuid"], "suggested_severity": "MEDIUM", "validation_status": "REFS_VERIFIED" },
  "latest_decision": { "id": "uuid", "outcome": "ALLOW", "policy_revision": "2026-11-06.1", "findings": [ ] },
  "available_actions": [ "RECONCILE_MERCHANT_ORDER" ],
  "pending_approvals": [ ]
}
```

Internal risk indicators, anomaly scores and raw policy input are never in the customer view.

## C3. Public endpoints

| Method and path | Caller | Purpose |
| --- | --- | --- |
| POST `/cases` | Customer | Open a case (with ownership proof) |
| GET `/cases` | Customer, merchant | List cases in caller scope |
| GET `/cases/{id}` | Party to the case | Role-scoped case view |
| GET `/cases/{id}/timeline` | Party | Audit projection filtered by visibility |
| GET `/cases/{id}/messages` | Party | Shared conversation |
| POST `/cases/{id}/messages` | Party | Add a message |
| POST `/cases/{id}/evidence` | Customer, merchant | Upload supporting evidence |
| GET `/cases/{id}/investigation` | Party | Simplified (customer) or detailed (merchant) |
| POST `/cases/{id}/investigation/runs` | Merchant | Re-run the agent |
| GET `/cases/{id}/decisions` | Merchant | Policy decisions with findings |
| POST `/cases/{id}/actions` | Merchant | Propose an action; evaluates policy; executes if ALLOW |
| GET `/cases/{id}/actions/{action_id}` | Merchant | Execution status |
| POST `/cases/{id}/approvals/{approval_id}/decision` | Merchant approver | Approve or reject |
| POST `/cases/{id}/approvals/{approval_id}/revoke` | Merchant approver | Revoke before execution |
| GET `/cases/{id}/events` | Party | Server-sent events for live updates |
| POST `/webhooks/paypal/{provider_account_id}` | PayPal | Webhook receiver (C5) |

### Authorization matrix

| Endpoint group | Customer | Merchant staff | Merchant admin | Agent service | Platform admin |
| --- | --- | --- | --- | --- | --- |
| Open or read own case | Yes | Merchant’s cases | Merchant’s cases | Read evidence only | Audit read |
| Messages and evidence | Yes | Yes | Yes | No | No |
| Re-run investigation | No | Yes | Yes | No | No |
| Propose action | No | Needs matching permission | Needs matching permission | No | No |
| Decide approval | No | No | Yes, and not the requester | No | No |
| Policy bundle admin | No | No | No | No | Yes (separate from merchant ops) |

## C4. Key endpoint contracts

### POST /cases

```json
{
  "merchant_id": "uuid",
  "merchant_order_ref": "ORDER-789",
  "claim": { "email": "buyer@example.com", "token": "opaque-from-confirmation-page" },
  "case_type": "PROVIDER_MERCHANT_STATE_MISMATCH",
  "description": "I was charged but the store says unpaid."
}
```

- 201 with `Location: /api/v1/cases/{id}` and the customer view. Idempotent on `Idempotency-Key`.
- If a live case already exists for that transaction and type, return 200 with the existing case and `"existing": true`. The customer joins the one shared case instead of creating a second.
- Ownership proof is checked against `order_claim_tokens` (hash compare, expiry, single use per case). Any failure returns the same generic 404 and writes a security event.
- Side effects in one transaction: create case, add audit event `CASE_OPENED`, write outbox row, then trigger evidence collection.

### POST /cases/{id}/actions

```json
{ "action_type": "RECONCILE_MERCHANT_ORDER", "params": { "amount_minor": 4900, "currency": "USD" } }
```

Server flow: authorize caller, fetch fresh PayPal and merchant state, build the OPA input, evaluate, store the decision, then branch.

```json
{
  "decision": { "id": "uuid", "outcome": "ALLOW", "policy_revision": "2026-11-06.1",
                "findings": [ { "rule_id": "PP-101", "code": "GATE_PASSED", "message": "Provider reports COMPLETED" } ],
                "missing_gates": [], "required_approvals": [] },
  "execution": { "id": "uuid", "status": "SUCCEEDED", "completed_at": "2026-11-06T10:00:03Z" },
  "approval": null,
  "case": { "status": "RESOLVED", "version": 6 }
}
```

| Outcome | HTTP | Body |
| --- | --- | --- |
| ALLOW and executed | 200 | `execution` filled |
| ALLOW, execution in progress | 202 | `execution.status` is IN\_PROGRESS |
| REQUIRE\_APPROVAL | 200 | `approval` filled (PENDING), case moves to AWAITING\_MERCHANT\_APPROVAL |
| WAIT | 200 | `missing_gates` and findings explain what evidence is needed |
| BLOCK | 200 | findings carry reason codes; nothing executed; audit event written |

If PayPal or OPA is unreachable, return 503 and execute nothing.

### POST /cases/{id}/approvals/{approval\_id}/decision

```json
{ "decision": "APPROVED", "reason": "Verified with customer" }
```

The server records the approval, re-fetches state, re-evaluates policy with the approval in `input.approvals`, and executes only if the new outcome is ALLOW. The response has the same shape as `POST /actions`. The approval is bound to `params_sha256`, so changed parameters need a new approval.

### GET /cases/{id}/events (SSE)

Events: `case.updated`, `message.created`, `decision.created`, `approval.requested`, `action.completed`. The payload carries only `{type, case_id, version}`. The client refetches its own projection, so events never leak role-restricted data.

## C5. Case state machine

| From | To | Trigger | Actor |
| --- | --- | --- | --- |
| (new) | OPEN | Case created | Customer or auto-reconciler |
| OPEN | COLLECTING\_EVIDENCE | Evidence collection starts | System |
| COLLECTING\_EVIDENCE | INVESTIGATING | Evidence normalized | System |
| INVESTIGATING | AWAITING\_CUSTOMER\_EVIDENCE | Decision WAIT needing customer input | Policy |
| AWAITING\_CUSTOMER\_EVIDENCE | COLLECTING\_EVIDENCE | Customer submits evidence | Customer |
| INVESTIGATING | AWAITING\_MERCHANT\_APPROVAL | Decision REQUIRE\_APPROVAL | Policy |
| AWAITING\_MERCHANT\_APPROVAL | ACTION\_PENDING | Approval granted and re-evaluation ALLOW | Merchant approver |
| AWAITING\_MERCHANT\_APPROVAL | REJECTED | Approval rejected | Merchant approver |
| INVESTIGATING | ACTION\_PENDING | Decision ALLOW | Policy |
| ACTION\_PENDING | RESOLVED | Execution SUCCEEDED | Executor |
| ACTION\_PENDING | ESCALATED | Execution FAILED or merchant 409 | Executor |
| INVESTIGATING | ESCALATED | Decision BLOCK | Policy |
| RESOLVED, ESCALATED, REJECTED | CLOSED | Closure | Merchant admin or system |

All transitions run in one transaction with the case row locked, bump `version` and write an audit event. Any other transition returns 409 `invalid-state`.

## C6. Webhook receiver contract

`POST /webhooks/paypal/{provider_account_id}`

1. Read the **raw body bytes** before any JSON parsing.
2. Read PayPal’s transmission headers (transmission ID, time, signature, certificate URL, auth algorithm; confirm exact names in PayPal’s webhook docs).
3. Verify using PayPal’s verify-webhook-signature call with the stored `webhook_id`, or local certificate verification.
4. Invalid: write `security_events` (`WEBHOOK_SIGNATURE_INVALID`), return 400. Nothing enters `webhook_events`.
5. Valid: `INSERT ... ON CONFLICT (provider, provider_event_id) DO NOTHING`. Inserted or duplicate, return 200 quickly. PayPal retries on non-2xx.
6. Hand the new event to an async worker that normalizes evidence and updates any matching case.

## C7. Internal contracts

### Go to agent

`POST {agent}/v1/investigate`

```json
{ "case_id": "uuid", "run_number": 1, "case_type": "PROVIDER_MERCHANT_STATE_MISMATCH", "customer_description": "..." }
```

### Agent tools (read-only, service token with scope `agent:read`)

| Endpoint | Returns |
| --- | --- |
| GET `/internal/v1/cases/{id}/evidence?type=` | Normalized evidence rows with IDs and provenance |
| GET `/internal/v1/cases/{id}/webhook-events` | Verified webhook history |
| GET `/internal/v1/telemetry/logs?case_id=&window=` | Elastic log search scoped to the case |
| GET `/internal/v1/telemetry/traces/{trace_id}` | Trace spans |
| GET `/internal/v1/telemetry/handler-error-rate?merchant_id=&window=` | Aggregate metric |

The agent has no PayPal credentials and no write endpoint except submitting its findings.

### Agent submission

`POST /internal/v1/cases/{id}/investigations`

```json
{
  "run_number": 1,
  "category": "PROVIDER_MERCHANT_STATE_MISMATCH",
  "root_cause_hypothesis": "Merchant webhook handler returned 500 after the capture event",
  "evidence_ids": ["uuid", "uuid"],
  "missing_evidence": [],
  "recommendation": "RECONCILE_MERCHANT_ORDER",
  "suggested_severity": "MEDIUM",
  "customer_explanation": "...",
  "merchant_fix_suggestion": "..."
}
```

Go validates before storing: JSON Schema, every `evidence_id` exists and belongs to this case, `recommendation` is in the action registry, text length limits. Failure returns 422 and stores the run as REJECTED with an audit event. A valid submission is stored as advisory only. It is later placed under `input.advisory`.

### Go to OPA

Input envelope and decision output are defined in A3 and A4. Endpoint: `POST /v1/data/fractal/guard/decision`. Timeout, non-200 or missing `result` means BLOCK.

## C8. Merchant integration contract

The endpoints FRACTAL calls on the merchant system. This is the provider-agnostic integration point. The mock store implements it for the demo.

**Signing:** headers `X-Fractal-Timestamp` and `X-Fractal-Signature` = hex HMAC-SHA256 of `timestamp + "." + raw_body` with a per-merchant secret. The merchant rejects timestamps more than 5 minutes old. Secrets are referenced by `integration_key_ref`, never stored in the database.

`GET /orders/{order_ref}`

```json
{ "order_ref": "ORDER-789", "status": "PAYMENT_PENDING", "amount_minor": 4900, "currency": "USD", "updated_at": "2026-11-06T09:41:00Z" }
```

`POST /orders/{order_ref}/reconcile` with header `Idempotency-Key`

```json
{ "provider": "PAYPAL", "provider_order_id": "PAYPAL-ORDER-123", "capture_id": "CAPTURE-456", "amount_minor": 4900, "currency": "USD", "fractal_case": "FR-10042" }
```

Response 200: `{ "order_ref": "ORDER-789", "status": "PAID", "reconciled_at": "...", "idempotent_replay": false }`.

- Same `Idempotency-Key`: return the same result with `idempotent_replay: true`.
- 409 if the order cannot be reconciled (for example CANCELLED). FRACTAL marks the execution FAILED and escalates the case.
- The merchant should also verify amount and currency itself.

## C9. Next deliverables

1. OpenAPI 3.1 file for C3 and C4, plus generated Go types and TypeScript client.
2. JSON Schemas for the OPA input envelope and the agent submission.
3. Migration files for Part B and the policy test fixtures for Part A.

*This is an engineering specification, not legal advice. Jurisdiction-specific policy packs need qualified legal review before production use.*
