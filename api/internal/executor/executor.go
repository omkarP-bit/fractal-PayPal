package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fractal/api/internal/evidence"
)

type Executor struct {
	db     *pgxpool.Pool
	payPal PayPalFetcher
	merch  MerchantAPI
	policy PolicyEvaluator
}

type PayPalFetcher interface {
	GetTransaction(ctx context.Context, providerOrderID string) (*evidence.Transaction, error)
}

type MerchantAPI interface {
	Reconcile(ctx context.Context, orderRef string, body []byte, idempotencyKey string) ([]byte, int, error)
	GetOrder(ctx context.Context, ref string) (OrderSnapshot, error)
}

type OrderSnapshot struct {
	Ref         string
	Status      string
	AmountMinor int64
	Currency    string
	FetchedAt   time.Time
}

type PolicyEvaluator interface {
	Evaluate(ctx context.Context, input []byte) ([]byte, error)
}

type ExecutionResult struct {
	ID          string
	Status      string
	Response    json.RawMessage
	Error       string
	CompletedAt time.Time
}

// Execute runs the full executor sequence: per-case lock, idempotency insert,
// fresh state validation, merchant reconcile call, and status update.
func (e *Executor) Execute(ctx context.Context, caseID, decisionID, actionType string, params json.RawMessage, idempotencyKey string) (*ExecutionResult, error) {
	tx, err := e.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Per-case advisory lock.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, caseID); err != nil {
		return nil, fmt.Errorf("advisory lock: %w", err)
	}

	// Idempotency insert — ON CONFLICT DO NOTHING. No row returned means duplicate.
	var execID string
	err = tx.QueryRow(ctx, `
		INSERT INTO action_executions (case_id, policy_decision_id, action_type, params, params_sha256, idempotency_key, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'IN_PROGRESS')
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id`,
		caseID, decisionID, actionType, string(params), sha256Hex(params), idempotencyKey,
	).Scan(&execID)
	if err == pgx.ErrNoRows {
		// Duplicate — load the stored result and return it without executing again.
		return e.loadExecution(ctx, tx, idempotencyKey)
	}
	if err != nil {
		return nil, fmt.Errorf("insert execution: %w", err)
	}

	// Fresh state validation — fail closed if PayPal is unreachable.
	if err := e.validateFreshState(ctx); err != nil {
		if err2 := e.markFailed(ctx, tx, execID, err.Error()); err2 != nil {
			return nil, err2
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &ExecutionResult{ID: execID, Status: "FAILED", Error: err.Error()}, nil
	}

	// Call merchant reconcile with the same idempotency key.
	body, statusCode, err := e.merch.Reconcile(ctx, "", params, idempotencyKey)
	if err != nil || statusCode >= 400 {
		if err2 := e.markFailed(ctx, tx, execID, err.Error()); err2 != nil {
			return nil, err2
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &ExecutionResult{ID: execID, Status: "FAILED", Error: err.Error()}, nil
	}

	if err := e.markSucceeded(ctx, tx, execID, body); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &ExecutionResult{
		ID:          execID,
		Status:      "SUCCEEDED",
		Response:    body,
		CompletedAt: time.Now().UTC(),
	}, nil
}

func (e *Executor) loadExecution(ctx context.Context, tx pgx.Tx, idempotencyKey string) (*ExecutionResult, error) {
	var id, status string
	var response, errMsg string
	var completedAt time.Time
	err := tx.QueryRow(ctx, `
		SELECT id, status, COALESCE(response, ''), COALESCE(error, ''), COALESCE(completed_at, '1970-01-01')
		FROM action_executions WHERE idempotency_key = $1`, idempotencyKey).
		Scan(&id, &status, &response, &errMsg, &completedAt)
	if err != nil {
		return nil, fmt.Errorf("load duplicate execution: %w", err)
	}
	return &ExecutionResult{
		ID:          id,
		Status:      status,
		Response:    json.RawMessage(response),
		Error:       errMsg,
		CompletedAt: completedAt,
	}, nil
}

func (e *Executor) markFailed(ctx context.Context, tx pgx.Tx, execID, errMsg string) error {
	_, err := tx.Exec(ctx, `UPDATE action_executions SET status = 'FAILED', error = $2, completed_at = now() WHERE id = $1`, execID, errMsg)
	return err
}

func (e *Executor) markSucceeded(ctx context.Context, tx pgx.Tx, execID string, response []byte) error {
	_, err := tx.Exec(ctx, `UPDATE action_executions SET status = 'SUCCEEDED', response = $2, completed_at = now() WHERE id = $1`, execID, string(response))
	return err
}

// validateFreshState confirms PayPal is reachable (fail closed).
func (e *Executor) validateFreshState(ctx context.Context) error {
	_, err := e.payPal.GetTransaction(ctx, "")
	if err != nil && !strings.Contains(err.Error(), "not found") {
		return fmt.Errorf("paypal unreachable: %w", err)
	}
	return nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
