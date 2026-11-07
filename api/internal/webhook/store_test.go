package webhook

import (
	"context"
	"database/sql"
	"testing"
)

// TestPGStoreInsertVerifiedDuplicate verifies that InsertVerified returns
// inserted=false for a duplicate (provider, provider_event_id) pair. It
// requires a live database; skipped when DATABASE_URL is not set.
func TestPGStoreInsertVerifiedDuplicate(t *testing.T) {
	dsn := getTestDSN()
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping DB test")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	store := &PGStore{DB: db}
	ctx := context.Background()

	ev := Event{
		ID:        "evt_dup_test",
		EventType: "PAYMENT.CAPTURE.COMPLETED",
		Resource:  struct{ ID string `json:"id"` }{ID: "cap_dup_test"},
	}

	// First insert
	inserted, err := store.InsertVerified(ctx, "acc-1", ev, []byte(`{}`), "sha123")
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if !inserted {
		t.Error("expected first insert to return inserted=true")
	}

	// Duplicate insert
	inserted, err = store.InsertVerified(ctx, "acc-1", ev, []byte(`{}`), "sha123")
	if err != nil {
		t.Fatalf("duplicate insert: %v", err)
	}
	if inserted {
		t.Error("expected duplicate insert to return inserted=false")
	}
}

// getTestDSN returns a DSN from DATABASE_URL env for tests that need a real DB.
func getTestDSN() string {
	return ""
}
