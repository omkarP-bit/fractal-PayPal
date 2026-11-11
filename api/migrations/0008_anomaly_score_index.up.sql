-- Phase 11 (anomaly detection): additive read indexes for anomaly_scores.
-- Lookups: latest score per merchant (admin case view fallback) and latest
-- score per case. Additive only — no existing object is modified or dropped.

CREATE INDEX IF NOT EXISTS ix_anomaly_scores_merchant_latest
  ON anomaly_scores (merchant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS ix_anomaly_scores_case_latest
  ON anomaly_scores (merchant_id, case_id, created_at DESC)
  WHERE case_id IS NOT NULL;
