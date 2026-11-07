package webhook

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"
)

// EventRow is a stored, verified event waiting to be processed.
type EventRow struct {
	ID         string
	EventType  string
	ResourceID string
	Payload    []byte
}

// Processor turns a verified event into domain effects (update payment_transactions,
// attach evidence to matching cases, and so on). It MUST be idempotent: a crash
// before commit means the same event is processed again.
type Processor interface {
	Process(ctx context.Context, ev EventRow) error
}

// NewWaker returns a non-blocking wake function for Handler.Wake and the channel
// for Worker.Wake.
func NewWaker() (func(), <-chan struct{}) {
	ch := make(chan struct{}, 1)
	return func() {
		select {
		case ch <- struct{}{}:
		default:
		}
	}, ch
}

// Worker drains webhook_events rows in RECEIVED state. FOR UPDATE SKIP LOCKED lets
// several workers run safely, and a crash rolls the row back to RECEIVED.
type Worker struct {
	DB   *sql.DB
	Proc Processor
	Wake <-chan struct{}
	Poll time.Duration // safety-net polling interval, default 5s
	Log  *slog.Logger
}

func (w *Worker) Run(ctx context.Context) {
	log := w.Log
	if log == nil {
		log = slog.Default()
	}
	poll := w.Poll
	if poll <= 0 {
		poll = 5 * time.Second
	}
	t := time.NewTicker(poll)
	defer t.Stop()
	for {
		for { // drain everything currently waiting
			found, err := w.processOne(ctx)
			if err != nil {
				log.Error("webhook worker", "err", err)
				break
			}
			if !found {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-w.Wake:
		case <-t.C:
		}
	}
}

func (w *Worker) processOne(ctx context.Context) (bool, error) {
	tx, err := w.DB.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var ev EventRow
	err = tx.QueryRowContext(ctx, `
		SELECT id, event_type, COALESCE(resource_id, ''), payload
		FROM webhook_events
		WHERE processing_status = 'RECEIVED'
		ORDER BY received_at
		LIMIT 1
		FOR UPDATE SKIP LOCKED`).Scan(&ev.ID, &ev.EventType, &ev.ResourceID, &ev.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	status := "PROCESSED"
	if perr := w.Proc.Process(ctx, ev); perr != nil {
		status = "FAILED" // TODO: add an attempts column and retry with backoff before FAILED
		slog.Error("event processing failed", "event", ev.ID, "err", perr)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE webhook_events SET processing_status = $2, processed_at = now() WHERE id = $1`,
		ev.ID, status); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
