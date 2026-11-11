# Phase 11 Anomaly Detection — Model Card

Operational anomaly scoring for FRACTAL. **Tighten-only**: a high score may
escalate a decision to `REQUIRE_APPROVAL` in OPA (`RSK-002`); it can never
turn `BLOCK` into `ALLOW`, never authorizes refund/reconciliation by itself,
and with the model disabled or the score `null` the system behaves exactly
as Phase 10.

## Pipeline

```
existing tables ──► features.py (5 features, per-merchant windows)
                        │
        ┌───────────────┼──────────────────┐
        ▼               ▼                  ▼
  baseline.py       iforest.py          vae.py (EXPERIMENTAL)
  (Model A)         (Model B)           (Model C)
        └─────── evaluate.py ─────────────┘
                        │  benchmarks on SYNTHETIC holdout
                        ▼
        api/internal/anomaly/artifacts/*.json  ◄── exported artifacts
                        │
                        ▼  (Go runtime: internal/anomaly)
        anomaly_scores table ──► risk.anomaly_score ──► OPA
```

## Feature log (grounded in existing tables)

| Feature | Source | Method | Missing semantics |
|---|---|---|---|
| `webhook_delivery_latency_p95_s` | `webhook_events.received_at/processed_at` | nearest-rank p95 | `null` when no samples |
| `webhook_retry_count` | `security_events` (`WEBHOOK_*`) + `webhook_events.processing_status='FAILED'` | count in window | `0.0` (observed absence) |
| `handler_error_rate` | `webhook_events` + `action_executions` | FAILED / total | `null` when no events |
| `capture_to_order_update_s` | `payment_transactions.last_fetched_at` → `order_snapshots.fetched_at` (same `merchant_order_ref`) | median lag | `null` when no pairs |
| `refund_velocity_per_hour` | `action_executions` `REFUND_PAYMENT` | count / window hours | `0.0` (observed absence) |

Known limitations:

- **No attempts/retry column exists** (`webhook/worker.go` TODO); rejected
  deliveries (`security_events`) are the persisted proxy for provider
  retries (any non-2xx makes PayPal redeliver).
- `payment_transactions` has no dedicated capture timestamp;
  `last_fetched_at` is the persisted observation time of captured state.
- Handler errors are logged, not metered per handler; `action_executions`
  FAILED status is the persisted proxy.
- All aggregation is per-merchant with an explicit tenant filter; records
  from another merchant are rejected (`TenantMismatchError`) or dropped.

## Models

### Model A — `baseline_thresholds` (transparent statistical thresholds)

- Per-feature Tukey upper fence: `Q3 + 1.5·IQR`, fitted on training vectors.
- Score = max over features of `clip((value − fence) / (hard_max − fence), 0, 1)`.
  Missing features contribute 0 (a missing signal never tightens alone).
- Documented hard caps (`HARD_MAX`): latency 900 s, retry count 50,
  error rate 1.0, capture lag 3600 s, refund velocity 50/h.
- The artifact contains every number used; an operator can reproduce any
  score by hand. Range `[0,1]`.

### Model B — `isolation_forest` (scikit-learn)

- `IsolationForest(n_estimators=100, contamination="auto")` on min-imputed
  training medians; trees exported as JSON (children/feature/threshold,
  per-node depth and average path length) for dependency-free Go inference.
- Score = `2^(−mean normalised path length)` ∈ (0,1], higher = more anomalous
  (the negation of sklearn's `score_samples`). Python/Go parity is asserted
  against sklearn's own scores to 1e-9 in `tests/test_iforest.py` and against
  golden fixtures in Go.

### Model C — `vae_logistic` (EXPERIMENTAL — not retained)

- VAE (5→16→2) with BCE reconstruction on min-max scaled inputs, plus a
  logistic classifier on `[reconstruction_error, z_mean]`.
- **Retention rule**: exported for runtime only if holdout ROC-AUC ≥ best
  baseline + 0.05 (`RETAIN_MARGIN`).
- **Current outcome on the synthetic benchmark: NOT retained**
  (baseline AUC 0.875, iforest AUC 1.0, VAE AUC 0.5 → below margin).
  No `vae.json` artifact is written; the Go runtime ships A + B only.
  torch is an optional dependency (tests skip when absent).

## Data provenance — SYNTHETIC

`app.anomaly.synthetic` generates labeled synthetic windows
(`data_origin: "SYNTHETIC"`, scenario tags: `normal`, `webhook_failure_burst`,
`latency_spike`, `handler_error_spike`, `capture_order_lag`, `refund_storm`).
The generator **refuses to overwrite any existing file** and never writes
into `api/scripts/seed*.sql` or Phase 10 scenario files.

> **Metrics produced by `evaluate.py` are measured on held-out SYNTHETIC
> data. They are not real-world accuracy and must never be presented as
> such.** Real-world validation requires sandbox/production telemetry.

## Evaluation (deterministic, seed=7)

Run `python -m app.anomaly.evaluate` from `investigator/`; writes
`evaluation_report.json` plus artifacts to `api/internal/anomaly/artifacts/`.

## Traceability

Every runtime score row (`anomaly_scores`) stores `model_id`
(e.g. `iforest-20261110.1`), the feature values + provenance JSON, and
`created_at`. The Go scorer returns model version, features and calculation
time with each score.

## Limitations

- Trained only on synthetic data; thresholds may not transfer to real traffic
  without refitting on observed telemetry (re-run evaluate with real exports).
- Windows are 24 h by default; short-lived anomalies inside a window are
  averaged into percentiles (p95 mitigates but does not eliminate this).
- No cross-window temporal model (no sequence model); velocity features are
  simple counts per window.
- Scores are per-merchant; they are not comparable across merchants with
  different traffic volume without normalisation.
