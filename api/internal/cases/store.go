package cases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fractal/api/internal/auth"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

type Case struct {
	ID               string
	CaseNumber       string
	MerchantID       string
	CustomerID       string
	MerchantOrderRef string
	CaseType         string
	Status           string
	Version          int
	// Anomaly is Phase 11 admin-only context. It is populated only for admin
	// callers on GET while the model is enabled; nil serializes as omitted,
	// so non-admin responses are unchanged from Phase 10.
	Anomaly *AnomalyInfo `json:"anomaly,omitempty"`
}

// AnomalyInfo is the admin-only anomaly score attached to a case view.
type AnomalyInfo struct {
	Score        float64 `json:"score"`
	ModelID      string  `json:"model_id"`
	ModelVersion string  `json:"model_version,omitempty"`
	WindowStart  string  `json:"window_start,omitempty"`
	WindowEnd    string  `json:"window_end,omitempty"`
	CalculatedAt string  `json:"calculated_at"`
	// Threshold mirrors policies/data.rego config.anomaly_approval_threshold
	// (display only; decisions read the Rego config). Scores at or above it
	// may escalate to REQUIRE_APPROVAL (RSK-002) — tighten-only, never a
	// path around BLOCK.
	Threshold float64             `json:"threshold"`
	Features  map[string]*float64 `json:"features,omitempty"`
}

// anomalyApprovalThreshold mirrors policies/data.rego
// config.anomaly_approval_threshold (0.8). Single source of truth for
// decisions stays in the Rego config; this copy is for operator display.
const anomalyApprovalThreshold = 0.8

// isParty returns true if the caller is the customer or a member of the case's merchant.
func (s *Store) isParty(ctx context.Context, caseID, callerKind, callerID, callerMerchantID string) (bool, error) {
	var merchantID, customerID string
	err := s.db.QueryRow(ctx, `SELECT merchant_id, customer_id FROM exception_cases WHERE id = $1`, caseID).
		Scan(&merchantID, &customerID)
	if err == pgx.ErrNoRows {
		return false, nil // not found -> treated as not a party (404, not 403)
	}
	if err != nil {
		return false, err
	}
	if callerKind == "CUSTOMER" && callerID == customerID {
		return true, nil
	}
	if callerKind == "MERCHANT_USER" || callerKind == "PLATFORM_ADMIN" {
		if callerMerchantID != "" && callerMerchantID == merchantID {
			return true, nil
		}
	}
	return false, nil
}

// GetCase fetches a case only if the caller is a party. Returns pgx.ErrNoRows-equivalent
// (nil, false) when the caller is not a party so handlers can return 404.
func (s *Store) GetCase(ctx context.Context, caseID string, claims *auth.Claims) (*Case, bool, error) {
	party, err := s.isParty(ctx, caseID, claims.Kind, claims.Sub, claims.MerchantID)
	if err != nil {
		return nil, false, err
	}
	if !party {
		return nil, false, nil
	}
	var c Case
	err = s.db.QueryRow(ctx, `
		SELECT id, case_number, merchant_id, customer_id, merchant_order_ref, case_type, status, version
		FROM exception_cases WHERE id = $1`, caseID).
		Scan(&c.ID, &c.CaseNumber, &c.MerchantID, &c.CustomerID, &c.MerchantOrderRef, &c.CaseType, &c.Status, &c.Version)
	if err != nil {
		return nil, false, err
	}
	return &c, true, nil
}

// LatestAnomalyScore returns the merchant's most recent anomaly score,
// preferring one recorded against this case, then any merchant-level score.
// Returns (nil, nil) when no score exists. Every query is scoped to
// merchantID, so another tenant's scores can never be read.
func (s *Store) LatestAnomalyScore(ctx context.Context, caseID, merchantID string) (*AnomalyInfo, error) {
	read := func(where string, args ...any) (*AnomalyInfo, error) {
		var (
			score    float64
			modelID  string
			features []byte
			created  time.Time
		)
		err := s.db.QueryRow(ctx, `
			SELECT score, model_id, features, created_at
			FROM anomaly_scores
			WHERE merchant_id = $1 AND `+where+`
			ORDER BY created_at DESC
			LIMIT 1`, args...).
			Scan(&score, &modelID, &features, &created)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return buildAnomalyInfo(score, modelID, features, created)
	}

	if caseID != "" {
		info, err := read("case_id = $2::uuid", merchantID, caseID)
		if err != nil || info != nil {
			return info, err
		}
	}
	return read("case_id IS NULL", merchantID)
}

// buildAnomalyInfo decodes the traceability payload stored in
// anomaly_scores.features (model version, window, feature values) written by
// scripts/anomaly. Missing/legacy payloads degrade to score + model only.
func buildAnomalyInfo(score float64, modelID string, features []byte, created time.Time) (*AnomalyInfo, error) {
	info := &AnomalyInfo{
		Score:        score,
		ModelID:      modelID,
		CalculatedAt: created.UTC().Format(time.RFC3339),
		Threshold:    anomalyApprovalThreshold,
	}
	if len(features) == 0 {
		return info, nil
	}
	var payload struct {
		ModelVersion string              `json:"model_version"`
		WindowStart  string              `json:"window_start"`
		WindowEnd    string              `json:"window_end"`
		Values       map[string]*float64 `json:"values"`
	}
	if err := json.Unmarshal(features, &payload); err != nil {
		return nil, fmt.Errorf("parse anomaly features jsonb: %w", err)
	}
	info.ModelVersion = payload.ModelVersion
	info.WindowStart = payload.WindowStart
	info.WindowEnd = payload.WindowEnd
	info.Features = payload.Values
	return info, nil
}

func (s *Store) ListCases(ctx context.Context, claims *auth.Claims) ([]Case, error) {
	var rows pgx.Rows
	var err error
	switch claims.Kind {
	case "CUSTOMER":
		rows, err = s.db.Query(ctx, `
			SELECT id, case_number, merchant_id, customer_id, merchant_order_ref, case_type, status, version
			FROM exception_cases WHERE customer_id = $1 ORDER BY created_at DESC`, claims.Sub)
	case "MERCHANT_USER", "PLATFORM_ADMIN":
		rows, err = s.db.Query(ctx, `
			SELECT id, case_number, merchant_id, customer_id, merchant_order_ref, case_type, status, version
			FROM exception_cases WHERE merchant_id = $1 ORDER BY created_at DESC`, claims.MerchantID)
	default:
		return nil, fmt.Errorf("unauthorized kind %s", claims.Kind)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Case
	for rows.Next() {
		var c Case
		if err := rows.Scan(&c.ID, &c.CaseNumber, &c.MerchantID, &c.CustomerID, &c.MerchantOrderRef, &c.CaseType, &c.Status, &c.Version); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
