-- Reverses 0008_anomaly_score_index.up.sql. Drops only the indexes added by
-- Phase 11; the anomaly_scores table and its rows are untouched.

DROP INDEX IF EXISTS ix_anomaly_scores_case_latest;
DROP INDEX IF EXISTS ix_anomaly_scores_merchant_latest;
