package fractal.fixtures

reconcile_ok := {
  "schema_version": "1.0",
  "request": {
    "evaluation_id": "11111111-1111-1111-1111-111111111111",
    "evaluated_at": "2026-11-07T10:00:00Z",
    "action": {
      "type": "RECONCILE_MERCHANT_ORDER",
      "params": {"amount_minor": 4900, "currency": "USD"},
      "params_sha256": "abc123",
      "idempotency_key": "idem-1"
    }
  },
  "case": {"id": "case-1", "case_number": "FR-10042", "status": "INVESTIGATING"},
  "actor": {"id": "user-1", "type": "MERCHANT_USER", "merchant_id": "MER-501", "permissions": ["reconcile"]},
  "merchant": {"id": "MER-501", "active": true},
  "facts": {
    "provider": {"name": "PAYPAL", "status": "COMPLETED", "amount_minor": 4900, "currency": "USD",
                 "source": "PAYPAL_API", "fetched_at": "2026-11-07T09:59:40Z"},
    "order": {"status": "PAYMENT_PENDING", "amount_minor": 4900, "currency": "USD",
              "source": "MERCHANT_API", "fetched_at": "2026-11-07T09:59:41Z"},
    "webhook": {"signature_verified": true, "event_count": 1}
  },
  "execution": {"already_executed": false},
  "risk": {"severity": "MEDIUM", "anomaly_score": null},
  "approvals": [],
  "applicability": [{"pack_id": "paypal_integration", "version": "1.0", "status": "APPLICABLE"}],
  "advisory": {"agent": {"recommendation": "RECONCILE_MERCHANT_ORDER", "confidence": 0.97}}
}

refund_no_approval := {
  "schema_version": "1.0",
  "request": {
    "evaluation_id": "22222222-2222-2222-2222-222222222222",
    "evaluated_at": "2026-11-07T10:00:00Z",
    "action": {
      "type": "REFUND_PAYMENT",
      "params": {"amount_minor": 4900, "currency": "USD"},
      "params_sha256": "def456",
      "idempotency_key": "idem-2"
    }
  },
  "case": {"id": "case-2", "case_number": "FR-10043", "status": "INVESTIGATING"},
  "actor": {"id": "user-2", "type": "MERCHANT_USER", "merchant_id": "MER-501", "permissions": ["refund"]},
  "merchant": {"id": "MER-501", "active": true},
  "facts": {
    "provider": {"name": "PAYPAL", "status": "COMPLETED", "amount_minor": 4900, "currency": "USD",
                 "source": "PAYPAL_API", "fetched_at": "2026-11-07T09:59:40Z"},
    "order": {"status": "PAYMENT_PENDING", "amount_minor": 4900, "currency": "USD",
              "source": "MERCHANT_API", "fetched_at": "2026-11-07T09:59:41Z"},
    "webhook": {"signature_verified": true, "event_count": 1}
  },
  "execution": {"already_executed": false},
  "risk": {"severity": "MEDIUM", "anomaly_score": null},
  "approvals": [],
  "applicability": [{"pack_id": "paypal_integration", "version": "1.0", "status": "APPLICABLE"}],
  "advisory": {"agent": {"recommendation": "REFUND_PAYMENT", "confidence": 0.9}}
}

agent_only := {
  "schema_version": "1.0",
  "request": {
    "evaluation_id": "33333333-3333-3333-3333-333333333333",
    "evaluated_at": "2026-11-07T10:00:00Z",
    "action": {
      "type": "RECONCILE_MERCHANT_ORDER",
      "params": {"amount_minor": 4900, "currency": "USD"},
      "params_sha256": "ghi789",
      "idempotency_key": "idem-3"
    }
  },
  "case": {"id": "case-3", "case_number": "FR-10044", "status": "INVESTIGATING"},
  "actor": {"id": "user-3", "type": "MERCHANT_USER", "merchant_id": "MER-501", "permissions": ["reconcile"]},
  "merchant": {"id": "MER-501", "active": true},
  "facts": {
    "provider": {"name": "PAYPAL", "status": "PENDING", "amount_minor": 4900, "currency": "USD",
                 "source": "PAYPAL_API", "fetched_at": "2026-11-07T09:00:00Z"},
    "order": {"status": "PAYMENT_PENDING", "amount_minor": 4900, "currency": "USD",
              "source": "MERCHANT_API", "fetched_at": "2026-11-07T09:59:41Z"},
    "webhook": {"signature_verified": false, "event_count": 0}
  },
  "execution": {"already_executed": false},
  "risk": {"severity": "LOW", "anomaly_score": null},
  "approvals": [],
  "applicability": [],
  "advisory": {"agent": {"recommendation": "RECONCILE_MERCHANT_ORDER", "confidence": 0.99}}
}
