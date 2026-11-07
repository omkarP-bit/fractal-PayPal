package approvals

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Manager struct {
	db     *pgxpool.Pool
	policy PolicyEvaluator
}

type PolicyEvaluator interface {
	Evaluate(ctx context.Context, input []byte) ([]byte, error)
}

type Executor interface {
	Execute(ctx context.Context, caseID, decisionID, actionType string, params json.RawMessage, idempotencyKey string) (ExecutionResult, error)
}

type ExecutionResult struct {
	ID     string
	Status string
}

// CreateApproval records a pending approval for a REQUIRE_APPROVAL decision.
func (m *Manager) CreateApproval(ctx context.Context, caseID, decisionID, actionType, paramsSHA256, requestedRole string, expiresAt time.Time) (string, error) {
	var id string
	err := m.db.QueryRow(ctx, `
		INSERT INTO approvals (case_id, policy_decision_id, action_type, params_sha256, requested_role, decision, expires_at)
		VALUES ($1, $2, $3, $4, $5, 'PENDING', $6)
		RETURNING id`,
		caseID, decisionID, actionType, paramsSHA256, requestedRole, expiresAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create approval: %w", err)
	}
	return id, nil
}

// Decision records an approve/reject decision on an approval. If approved and
// re-evaluation yields ALLOW, executes the action.
func (m *Manager) Decision(ctx context.Context, approvalID, approverID, decision, reason string) (ExecutionResult, error) {
	tx, err := m.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Lock the approval row.
	var caseID, decisionID, actionType, paramsSHA256, requestedRole string
	var expiresAt time.Time
	var currentDecision string
	err = tx.QueryRow(ctx, `
		SELECT case_id, policy_decision_id, action_type, params_sha256, requested_role, decision, expires_at
		FROM approvals WHERE id = $1 FOR UPDATE`, approvalID).
		Scan(&caseID, &decisionID, &actionType, &paramsSHA256, &requestedRole, &currentDecision, &expiresAt)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("load approval: %w", err)
	}

	if currentDecision != "PENDING" {
		return ExecutionResult{}, fmt.Errorf("approval already decided: %s", currentDecision)
	}
	if time.Now().After(expiresAt) {
		return ExecutionResult{}, fmt.Errorf("approval expired")
	}

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `
		UPDATE approvals SET decision = $1, reason = $2, decided_at = $3 WHERE id = $4`,
		decision, reason, now, approvalID)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("update approval: %w", err)
	}

	if decision == "REJECTED" {
		if err := tx.Commit(ctx); err != nil {
			return ExecutionResult{}, err
		}
		return ExecutionResult{Status: "REJECTED"}, nil
	}

	// APPROVED: re-evaluate policy with the approval in input.approvals.
	// Execute only if re-evaluation yields ALLOW.
	result, err := m.executeIfAllowed(ctx, tx, caseID, decisionID, actionType, paramsSHA256)
	if err != nil {
		return ExecutionResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ExecutionResult{}, err
	}
	return result, nil
}

// Revoke revokes a pending approval before execution.
func (m *Manager) Revoke(ctx context.Context, approvalID string) error {
	_, err := m.db.Exec(ctx, `
		UPDATE approvals SET revoked_at = now() WHERE id = $1 AND decision = 'PENDING'`,
		approvalID)
	if err != nil {
		return fmt.Errorf("revoke approval: %w", err)
	}
	return nil
}

func (m *Manager) executeIfAllowed(ctx context.Context, tx pgx.Tx, caseID, decisionID, actionType, paramsSHA256 string) (ExecutionResult, error) {
	// Placeholder: re-evaluate policy and execute via Executor if ALLOW.
	// Wired in Phase 8 integration with the real OPA client and Executor.
	return ExecutionResult{Status: "PENDING_REEVALUATION"}, nil
}

var _ = sha256.Sum256
var _ = hex.EncodeToString
