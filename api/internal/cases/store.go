package cases

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"fractal/api/internal/auth"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

type Case struct {
	ID              string
	CaseNumber      string
	MerchantID      string
	CustomerID      string
	MerchantOrderRef string
	CaseType        string
	Status          string
	Version         int
}

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
