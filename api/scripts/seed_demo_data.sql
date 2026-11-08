-- Demo data for FRACTAL hackathon demonstration
-- Run: psql "$DATABASE_URL" -f scripts/seed_demo_data.sql

-- Insert demo payment transaction
INSERT INTO payment_transactions (merchant_id, provider_account_id, provider, provider_order_id, capture_id, amount_minor, currency, provider_status, merchant_order_ref, last_fetched_at)
VALUES (
  '00000000-0000-0000-0000-0000000000a1',
  '00000000-0000-0000-0000-0000000000b1',
  'PAYPAL',
  'PAYPAL-ORDER-123',
  'CAPTURE-456',
  4900,
  'USD',
  'COMPLETED',
  'ORDER-789',
  now()
)
ON CONFLICT (provider, provider_order_id) DO NOTHING;

-- Insert demo exception case
INSERT INTO exception_cases (merchant_id, customer_id, payment_transaction_id, merchant_order_ref, case_type, status, opened_via, version)
VALUES (
  '00000000-0000-0000-0000-0000000000a1',
  '00000000-0000-0000-0000-0000000000c1',
  (SELECT id FROM payment_transactions WHERE provider_order_id = 'PAYPAL-ORDER-123'),
  'ORDER-789',
  'PROVIDER_MERCHANT_STATE_MISMATCH',
  'INVESTIGATING',
  'CUSTOMER',
  1
)
ON CONFLICT (payment_transaction_id, case_type) WHERE status NOT IN ('RESOLVED','REJECTED','CLOSED') DO NOTHING;

-- Insert demo webhook event
INSERT INTO webhook_events (provider, provider_event_id, provider_account_id, event_type, resource_id, signature_verified, payload, payload_sha256, received_at, processing_status)
VALUES (
  'PAYPAL',
  'evt_CAPTURE-456',
  '00000000-0000-0000-0000-0000000000b1',
  'PAYMENT.CAPTURE.COMPLETED',
  'CAPTURE-456',
  true,
  '{"id":"evt_CAPTURE-456","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"CAPTURE-456","status":"COMPLETED","amount":{"value":"49.00","currency":"USD"}}}',
  encode(sha256('{"id":"evt_CAPTURE-456","event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"CAPTURE-456","status":"COMPLETED","amount":{"value":"49.00","currency":"USD"}}}')::bytea, 'hex'),
  now(),
  'PROCESSED'
)
ON CONFLICT (provider, provider_event_id) DO NOTHING;

-- Insert case evidence
INSERT INTO case_evidence (case_id, evidence_type, source_system, source_ref, fetched_at, payload, payload_sha256)
SELECT
  c.id,
  'PROVIDER_TRANSACTION',
  'PAYPAL_API',
  'paypal:PAYPAL-ORDER-123',
  now(),
  '{"provider_order_id":"PAYPAL-ORDER-123","capture_id":"CAPTURE-456","status":"COMPLETED","amount_minor":4900,"currency":"USD"}',
  encode(sha256('{"provider_order_id":"PAYPAL-ORDER-123","capture_id":"CAPTURE-456","status":"COMPLETED","amount_minor":4900,"currency":"USD"}')::bytea, 'hex')
FROM exception_cases c
WHERE c.case_number = 'FR-10042'
ON CONFLICT DO NOTHING;

-- Insert policy decision
INSERT INTO policy_decisions (case_id, evaluation_id, action_type, params_sha256, input, input_sha256, outcome, findings, policy_revision, requested_by)
SELECT
  c.id,
  '00000000-0000-0000-0000-0000000000d1',
  'RECONCILE_MERCHANT_ORDER',
  encode(sha256('{"amount_minor":4900,"currency":"USD"}')::bytea, 'hex'),
  '{"schema_version":"1.0","request":{"action":{"type":"RECONCILE_MERCHANT_ORDER","params":{"amount_minor":4900,"currency":"USD"}}}}',
  encode(sha256('{"schema_version":"1.0","request":{"action":{"type":"RECONCILE_MERCHANT_ORDER","params":{"amount_minor":4900,"currency":"USD"}}}}')::bytea, 'hex'),
  'ALLOW',
  '[{"rule_id":"PP-101","code":"GATE_PASSED","message":"Provider reports COMPLETED"},{"rule_id":"PP-102","code":"GATE_PASSED","message":"Amount and currency match"}]',
  '2026-11-06.1',
  '00000000-0000-0000-0000-0000000000c1'
FROM exception_cases c
WHERE c.case_number = 'FR-10042'
ON CONFLICT (evaluation_id) DO NOTHING;

-- Insert action execution
INSERT INTO action_executions (case_id, policy_decision_id, action_type, params, params_sha256, idempotency_key, status, response, started_at, completed_at)
SELECT
  c.id,
  d.id,
  'RECONCILE_MERCHANT_ORDER',
  '{"amount_minor":4900,"currency":"USD"}',
  encode(sha256('{"amount_minor":4900,"currency":"USD"}')::bytea, 'hex'),
  'idem-frac-10042-001',
  'SUCCEEDED',
  '{"order_ref":"ORDER-789","status":"PAID","reconciled_at":"2026-11-06T10:05:00Z"}',
  now(),
  now()
FROM exception_cases c
JOIN policy_decisions d ON d.case_id = c.id
WHERE c.case_number = 'FR-10042'
ON CONFLICT (idempotency_key) DO NOTHING;

-- Insert audit events
INSERT INTO audit_events (case_id, actor_type, actor_id, event_type, visibility, data, prev_hash, event_hash, created_at)
SELECT
  c.id,
  'CUSTOMER',
  '00000000-0000-0000-0000-0000000000c1',
  'CASE_OPENED',
  'CUSTOMER',
  '{"case_number":"FR-10042","merchant_order_ref":"ORDER-789"}',
  '',
  encode(sha256(('CASE_OPENED' || '00000000-0000-0000-0000-0000000000c1')::bytea), 'hex'),
  now() - interval '5 minutes'
FROM exception_cases c
WHERE c.case_number = 'FR-10042'
ON CONFLICT DO NOTHING;

INSERT INTO audit_events (case_id, actor_type, actor_id, event_type, visibility, data, prev_hash, event_hash, created_at)
SELECT
  c.id,
  'SYSTEM',
  'system',
  'CASE_RECONCILED',
  'INTERNAL',
  '{"action_type":"RECONCILE_MERCHANT_ORDER","outcome":"ALLOW","policy_revision":"2026-11-06.1"}',
  (SELECT event_hash FROM audit_events WHERE case_id = c.id ORDER BY created_at DESC LIMIT 1),
  encode(sha256(('CASE_RECONCILED' || coalesce((SELECT event_hash FROM audit_events WHERE case_id = c.id ORDER BY created_at DESC LIMIT 1),''))::bytea), 'hex'),
  now()
FROM exception_cases c
WHERE c.case_number = 'FR-10042'
ON CONFLICT DO NOTHING;

-- Update case status
UPDATE exception_cases
SET status = 'RESOLVED', resolution_code = 'RECONCILED', closed_at = now(), version = version + 1
WHERE case_number = 'FR-10042';
