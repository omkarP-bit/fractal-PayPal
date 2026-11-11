package reconciler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	StaleAfter        time.Duration
	PollInterval      time.Duration
	MerchantID        string
	SystemCustomerID  string
}

func (c Config) withDefaults() Config {
	if c.StaleAfter <= 0 {
		c.StaleAfter = 15 * time.Minute
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 30 * time.Second
	}
	return c
}

type Job struct {
	db  *pgxpool.Pool
	log *slog.Logger
	cfg Config
}

func New(db *pgxpool.Pool, log *slog.Logger, cfg Config) *Job {
	return &Job{db: db, log: log, cfg: cfg.withDefaults()}
}

func (j *Job) Run(ctx context.Context) error {
	candidates, err := j.stalePayments(ctx)
	if err != nil {
		return fmt.Errorf("list stale payments: %w", err)
	}
	for _, c := range candidates {
		if err := j.openCase(ctx, c); err != nil {
			j.log.Warn("proactive reconciler: could not open case",
				"provider_order_id", c.providerOrderID, "err", err)
			continue
		}
	}
	return nil
}

func (j *Job) RunLoop(ctx context.Context) error {
	if err := j.Run(ctx); err != nil {
		j.log.Error("proactive reconciler scan", "err", err)
	}
	t := time.NewTicker(j.cfg.PollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if err := j.Run(ctx); err != nil {
				j.log.Error("proactive reconciler scan", "err", err)
			}
		}
	}
}

type stalePayment struct {
	merchantID         string
	providerOrderID    string
	providerAccountID  string
	paymentID          string
	merchantOrderRef   string
	amountMinor        int64
	currency           string
}

func (j *Job) stalePayments(ctx context.Context) ([]stalePayment, error) {
	args := []any{time.Now().UTC().Add(-j.cfg.StaleAfter)}
	merchantFilter := ""
	if j.cfg.MerchantID != "" {
		merchantFilter = "AND pt.merchant_id = $" + string(rune(len(args)+1))
		args = append(args, j.cfg.MerchantID)
	}
	query := fmt.Sprintf(`
		SELECT pt.merchant_id::text, pt.provider_order_id,
		       pt.provider_account_id::text, pt.id::text,
		       pt.merchant_order_ref, pt.amount_minor, pt.currency
		FROM payment_transactions pt
		WHERE pt.provider_status = 'COMPLETED'
		  AND pt.last_fetched_at < $1
		  %s
		  AND NOT EXISTS (
		    SELECT 1 FROM order_snapshots os
		    WHERE os.merchant_id = pt.merchant_id
		      AND os.merchant_order_ref = pt.merchant_order_ref
		      AND os.order_status = 'PAID'
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM exception_cases c
		    WHERE c.payment_transaction_id = pt.id
		      AND c.case_type = 'PROVIDER_MERCHANT_STATE_MISMATCH'
		      AND c.status NOT IN ('RESOLVED','REJECTED','CLOSED')
		  )
		ORDER BY pt.last_fetched_at ASC`, merchantFilter)

	rows, err := j.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []stalePayment
	for rows.Next() {
		var c stalePayment
		if err := rows.Scan(&c.merchantID, &c.providerOrderID, &c.providerAccountID,
			&c.paymentID, &c.merchantOrderRef, &c.amountMinor, &c.currency); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (j *Job) openCase(ctx context.Context, c stalePayment) error {
	tx, err := j.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
		"case:"+c.paymentID); err != nil {
		return err
	}

	var exists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM exception_cases
			WHERE payment_transaction_id = $1
			  AND case_type = 'PROVIDER_MERCHANT_STATE_MISMATCH'
			  AND status NOT IN ('RESOLVED','REJECTED','CLOSED')
		)`, c.paymentID).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	if j.cfg.SystemCustomerID == "" {
		j.log.Warn("proactive reconciler: SystemCustomerID not set; skipping case creation",
			"provider_order_id", c.providerOrderID)
		return nil
	}

	var caseID, caseNumber string
	err = tx.QueryRow(ctx, `
		INSERT INTO exception_cases
			(merchant_id, customer_id, payment_transaction_id, merchant_order_ref,
			 case_type, status, opened_via, version)
		VALUES ($1, $2, $3, $4, 'PROVIDER_MERCHANT_STATE_MISMATCH', 'OPEN',
		        'AUTO_RECONCILER', 1)
		RETURNING id::text, case_number`,
		c.merchantID, j.cfg.SystemCustomerID,
		c.paymentID, c.merchantOrderRef).Scan(&caseID, &caseNumber)
	if err != nil {
		return err
	}

	_, _ = tx.Exec(ctx, `
		INSERT INTO audit_events (case_id, actor_type, actor_id, event_type, visibility, data, event_hash)
		VALUES ($1, 'SYSTEM', 'system', 'CASE_OPENED', 'INTERNAL', $2, $3)`,
		caseID,
		fmt.Sprintf(`{"case_number":%q,"opened_via":"AUTO_RECONCILER","provider_order_id":%q}`,
			caseNumber, c.providerOrderID),
		sha256Hex([]byte(fmt.Sprintf("CASE_OPENED:AUTO:%s", caseID))))

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	j.log.Info("proactive reconciler opened case",
		"case_id", caseID, "case_number", caseNumber,
		"provider_order_id", c.providerOrderID)
	return nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
