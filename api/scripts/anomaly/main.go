//go:build anomaly

// Command anomaly scores merchant telemetry windows with the shipped
// Phase 11 models and writes rows to anomaly_scores.
//
// This is an offline analysis tool: writing a score never changes a
// decision. risk.anomaly_score only reaches OPA when the API runs with
// FRACTAL_ANOMALY_ENABLED=true (and even then a missing score is null =
// Phase 10 behavior).
//
// Usage (from api/):
//
//	go run -tags anomaly ./scripts/anomaly [-model iforest|baseline] \
//	    [-window-hours 24] [-merchant <uuid>] [-dry-run]
//
// Scoring reads whatever telemetry exists in the connected database.
// Evaluation metrics produced by investigator/app/anomaly/evaluate.py are
// measured on SYNTHETIC data and are labeled as such in
// internal/anomaly/artifacts/evaluation_report.json — they are not
// real-world accuracy.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fractal/api/internal/anomaly"
)

func main() {
	model := flag.String("model", "iforest", "model to score with: iforest | baseline")
	windowHours := flag.Float64("window-hours", 24, "aggregation window length in hours")
	merchant := flag.String("merchant", "", "score only this merchant id (default: all merchants)")
	dryRun := flag.Bool("dry-run", false, "print scores without writing anomaly_scores")
	flag.Parse()

	if *windowHours <= 0 {
		fatal("-window-hours must be > 0")
	}
	var modelSel anomaly.ScoreModel
	switch *model {
	case "iforest":
		modelSel = anomaly.UseIForest
	case "baseline":
		modelSel = anomaly.UseBaseline
	default:
		fatal(fmt.Sprintf("unknown -model %q (want iforest or baseline)", *model))
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fatal("DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		fatal(fmt.Sprintf("connect db: %v", err))
	}
	defer pool.Close()

	// The CLI scores explicitly; the decision path stays gated separately by
	// FRACTAL_ANOMALY_ENABLED (see cmd/fractal-api/main.go).
	scorer, err := anomaly.NewScorer(true)
	if err != nil {
		fatal(fmt.Sprintf("load shipped artifacts: %v", err))
	}
	switch os.Getenv("FRACTAL_ANOMALY_ENABLED") {
	case "true", "1", "yes":
	default:
		fmt.Println("note: FRACTAL_ANOMALY_ENABLED is off — scores are recorded for analysis only; risk.anomaly_score stays null")
	}

	ctx := context.Background()
	end := time.Now().UTC().Truncate(time.Second)
	start := end.Add(-time.Duration(*windowHours * float64(time.Hour)))
	window := anomaly.Window{Start: start, End: end}

	merchants, err := merchantIDs(ctx, pool, *merchant)
	if err != nil {
		fatal(err.Error())
	}
	if len(merchants) == 0 {
		fmt.Println("no merchants to score")
		return
	}

	baselineVer, iforestVer := scorer.ModelVersions()
	fmt.Printf("model=%s baseline=%s iforest=%s window=%s..%s (%gh) dry_run=%v\n",
		*model, baselineVer, iforestVer,
		window.Start.Format(time.RFC3339), window.End.Format(time.RFC3339),
		*windowHours, *dryRun)

	failed := false
	for _, merchantID := range merchants {
		te, err := loadTelemetry(ctx, pool, merchantID, window)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: merchant %s: load telemetry: %v\n", merchantID, err)
			failed = true
			continue
		}
		fv, err := anomaly.Extract(te)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: merchant %s: extract features: %v\n", merchantID, err)
			failed = true
			continue
		}
		score, err := scorer.Score(modelSel, fv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: merchant %s: score: %v\n", merchantID, err)
			failed = true
			continue
		}

		// features jsonb = feature values + traceability (model version,
		// window, provenance, imputation, calculation time).
		payload, err := json.Marshal(map[string]any{
			"model_version":    score.ModelVersion,
			"model_type":       score.ModelType,
			"calculated_at":    score.CalculatedAt,
			"window_start":     fv.WindowStart,
			"window_end":       fv.WindowEnd,
			"values":           fv.Values,
			"provenance":       fv.Provenance,
			"imputed_features": score.Imputed,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: merchant %s: marshal features: %v\n", merchantID, err)
			failed = true
			continue
		}

		if !*dryRun {
			if _, err := pool.Exec(ctx, `
				INSERT INTO anomaly_scores (merchant_id, case_id, model_id, score, features)
				VALUES ($1, NULL, $2, $3, $4::jsonb)`,
				merchantID, score.ModelID, *score.Score, string(payload)); err != nil {
				fmt.Fprintf(os.Stderr, "error: merchant %s: insert score: %v\n", merchantID, err)
				failed = true
				continue
			}
		}

		line, _ := json.Marshal(map[string]any{
			"merchant_id": merchantID,
			"model_id":    score.ModelID,
			"score":       *score.Score,
			"dry_run":     *dryRun,
			"features":    fv.Values,
			"window":      []string{fv.WindowStart, fv.WindowEnd},
		})
		fmt.Println(string(line))
	}
	if failed {
		os.Exit(1)
	}
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(1)
}

// merchantIDs returns the merchants to score: exactly the requested one, or
// every merchant in the table. Tenant scoping is per-merchant from here on.
func merchantIDs(ctx context.Context, pool *pgxpool.Pool, requested string) ([]string, error) {
	if requested != "" {
		return []string{requested}, nil
	}
	rows, err := pool.Query(ctx, `SELECT id::text FROM merchants ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list merchants: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// loadTelemetry reads one merchant's raw rows for the window. Every query is
// scoped to merchantID (no cross-tenant mixing); security events are
// attributed to the merchant through detail.account -> provider_accounts.
// Rows without a resolvable account are not attributed (documented
// limitation: see investigator/app/anomaly/README.md).
func loadTelemetry(ctx context.Context, pool *pgxpool.Pool, merchantID string, w anomaly.Window) (anomaly.Telemetry, error) {
	te := anomaly.Telemetry{MerchantID: merchantID, Window: w}

	// webhook_events -> delivery latency + failed processing.
	rows, err := pool.Query(ctx, `
		SELECT we.received_at, we.processed_at, we.processing_status
		FROM webhook_events we
		JOIN provider_accounts pa ON pa.id = we.provider_account_id
		WHERE pa.merchant_id = $1 AND we.received_at >= $2 AND we.received_at < $3`,
		merchantID, w.Start, w.End)
	if err != nil {
		return te, fmt.Errorf("webhook_events: %w", err)
	}
	for rows.Next() {
		var received time.Time
		var processed *time.Time
		var status string
		if err := rows.Scan(&received, &processed, &status); err != nil {
			rows.Close()
			return te, fmt.Errorf("webhook_events scan: %w", err)
		}
		te.WebhookEvents = append(te.WebhookEvents, anomaly.WebhookEvent{
			MerchantID: merchantID, ReceivedAt: received, ProcessedAt: processed, ProcessingStatus: status,
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return te, fmt.Errorf("webhook_events: %w", err)
	}

	// security_events (WEBHOOK_* rejections) -> retry proxy, via account.
	rows, err = pool.Query(ctx, `
		SELECT se.event_type, se.created_at
		FROM security_events se
		WHERE se.created_at >= $2 AND se.created_at < $3
		  AND substr(se.event_type, 1, 8) = 'WEBHOOK_'
		  AND EXISTS (
		    SELECT 1 FROM provider_accounts pa
		    WHERE pa.merchant_id = $1 AND se.detail->>'account' = pa.id::text)`,
		merchantID, w.Start, w.End)
	if err != nil {
		return te, fmt.Errorf("security_events: %w", err)
	}
	for rows.Next() {
		var eventType string
		var created time.Time
		if err := rows.Scan(&eventType, &created); err != nil {
			rows.Close()
			return te, fmt.Errorf("security_events scan: %w", err)
		}
		te.SecurityEvents = append(te.SecurityEvents, anomaly.SecurityEvent{
			MerchantID: merchantID, EventType: eventType, CreatedAt: created,
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return te, fmt.Errorf("security_events: %w", err)
	}

	// action_executions (via case) -> handler error rate + refund velocity.
	rows, err = pool.Query(ctx, `
		SELECT ae.action_type, ae.status, ae.started_at
		FROM action_executions ae
		JOIN exception_cases ec ON ec.id = ae.case_id
		WHERE ec.merchant_id = $1 AND ae.started_at >= $2 AND ae.started_at < $3`,
		merchantID, w.Start, w.End)
	if err != nil {
		return te, fmt.Errorf("action_executions: %w", err)
	}
	for rows.Next() {
		var actionType, status string
		var started time.Time
		if err := rows.Scan(&actionType, &status, &started); err != nil {
			rows.Close()
			return te, fmt.Errorf("action_executions scan: %w", err)
		}
		te.ActionExecutions = append(te.ActionExecutions, anomaly.ActionExecution{
			MerchantID: merchantID, ActionType: actionType, Status: status, StartedAt: started,
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return te, fmt.Errorf("action_executions: %w", err)
	}

	// payment_transactions (captured) + order_snapshots -> capture lag.
	rows, err = pool.Query(ctx, `
		SELECT p.capture_id, p.merchant_order_ref, p.last_fetched_at
		FROM payment_transactions p
		WHERE p.merchant_id = $1 AND p.last_fetched_at >= $2 AND p.last_fetched_at < $3`,
		merchantID, w.Start, w.End)
	if err != nil {
		return te, fmt.Errorf("payment_transactions: %w", err)
	}
	for rows.Next() {
		var captureID *string
		var ref string
		var fetched time.Time
		if err := rows.Scan(&captureID, &ref, &fetched); err != nil {
			rows.Close()
			return te, fmt.Errorf("payment_transactions scan: %w", err)
		}
		te.PaymentTransactions = append(te.PaymentTransactions, anomaly.PaymentTransaction{
			MerchantID: merchantID, CaptureID: captureID, MerchantOrderRef: ref, LastFetchedAt: fetched,
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return te, fmt.Errorf("payment_transactions: %w", err)
	}

	rows, err = pool.Query(ctx, `
		SELECT o.merchant_order_ref, o.fetched_at
		FROM order_snapshots o
		WHERE o.merchant_id = $1 AND o.fetched_at >= $2 AND o.fetched_at < $3`,
		merchantID, w.Start, w.End)
	if err != nil {
		return te, fmt.Errorf("order_snapshots: %w", err)
	}
	for rows.Next() {
		var ref string
		var fetched time.Time
		if err := rows.Scan(&ref, &fetched); err != nil {
			rows.Close()
			return te, fmt.Errorf("order_snapshots scan: %w", err)
		}
		te.OrderSnapshots = append(te.OrderSnapshots, anomaly.OrderSnapshot{
			MerchantID: merchantID, MerchantOrderRef: ref, FetchedAt: fetched,
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return te, fmt.Errorf("order_snapshots: %w", err)
	}

	return te, nil
}
