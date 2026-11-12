// Command migrate applies the embedded SQL migrations to the database.
//
// It is safe to run on every boot: a schema_migrations ledger records applied
// versions and a Postgres advisory lock serialises concurrent instances.
//
// Databases created by the pre-migration Makefile target have the full schema
// but no ledger. The first run baselines such a database by recording every
// migration as applied instead of replaying SQL that is not idempotent.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fractal/api/migrations"
)

// advisoryLockKey is an arbitrary constant shared by every API instance so
// only one of them migrates at a time.
const advisoryLockKey int64 = 918273645

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		slog.Error("DATABASE_URL is required to run migrations")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, err := connect(ctx, dsn)
	if err != nil {
		slog.Error("connect to database", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := run(ctx, pool); err != nil {
		slog.Error("migration failed", "err", err)
		os.Exit(1)
	}
	slog.Info("migrations up to date")
}

// connect retries so a freshly provisioned database that is still accepting
// connections does not fail the deploy.
func connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	var lastErr error
	for attempt := 1; attempt <= 30; attempt++ {
		pool, err := pgxpool.New(ctx, dsn)
		if err == nil {
			if err = pool.Ping(ctx); err == nil {
				return pool, nil
			}
			pool.Close()
		}
		lastErr = err
		if attempt == 30 {
			break
		}
		slog.Warn("database not ready, retrying", "attempt", attempt, "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, lastErr
}

func run(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		// Use a background context so the unlock still runs if the caller's
		// context was cancelled.
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(unlockCtx, "SELECT pg_advisory_unlock($1)", advisoryLockKey); err != nil {
			slog.Warn("release migration lock", "err", err)
		}
	}()

	if _, err := conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version text PRIMARY KEY,
			applied_at timestamptz NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}

	all, err := migrations.Up()
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}

	if len(applied) == 0 {
		legacy, err := hasLegacySchema(ctx, conn)
		if err != nil {
			return err
		}
		if legacy {
			slog.Warn("existing schema without a migration ledger; recording a baseline")
			if err := baseline(ctx, conn, all); err != nil {
				return err
			}
			return nil
		}
	}

	for _, migration := range all {
		if applied[migration.Version] {
			continue
		}
		if err := apply(ctx, conn, migration); err != nil {
			return err
		}
		slog.Info("applied migration", "version", migration.Version)
	}
	return nil
}

func appliedVersions(ctx context.Context, conn *pgxpool.Conn) (map[string]bool, error) {
	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("read migration ledger: %w", err)
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan migration ledger: %w", err)
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read migration ledger: %w", err)
	}
	return applied, nil
}

// hasLegacySchema reports whether the database was set up before the
// schema_migrations ledger existed.
func hasLegacySchema(ctx context.Context, conn *pgxpool.Conn) (bool, error) {
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT to_regclass('public.users') IS NOT NULL").Scan(&exists); err != nil {
		return false, fmt.Errorf("check for existing schema: %w", err)
	}
	return exists, nil
}

func baseline(ctx context.Context, conn *pgxpool.Conn, all []migrations.Migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin baseline transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, migration := range all {
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING", migration.Version); err != nil {
			return fmt.Errorf("record baseline %s: %w", migration.Version, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit baseline: %w", err)
	}
	slog.Info("baselined existing schema", "migrations", len(all))
	return nil
}

func apply(ctx context.Context, conn *pgxpool.Conn, migration migrations.Migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin %s: %w", migration.Version, err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, migration.SQL); err != nil {
		return fmt.Errorf("apply %s: %w", migration.Version, err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", migration.Version); err != nil {
		return fmt.Errorf("record %s: %w", migration.Version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s: %w", migration.Version, err)
	}
	return nil
}
