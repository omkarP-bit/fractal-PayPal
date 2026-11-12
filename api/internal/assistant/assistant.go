// Package assistant implements the FRACTAL AI Case Assistant.
//
// Three capabilities live here:
//
//  1. Incident grouping — cases that share verified facts (same root cause,
//     same provider account, same webhook signal) inside a time window are
//     clustered into one incident. The Go backend verifies the shared facts
//     before a case is accepted into an incident; the AI never asserts a
//     grouping on its own authority.
//
//  2. Triage inbox — incidents and their cases are ranked by a priority score
//     computed from verified facts only (severity from the risk engine,
//     payment amount, customer wait time, incident size).
//
//  3. Drafted replies — customer replies are rendered from templates filled
//     with verified case data. The assistant cannot promise refunds,
//     compensation, timelines or outcomes, and every generated message is
//     labeled AI-assisted and written to the audit timeline.
//
// The assistant has no provider credentials and cannot execute actions.
// Incident membership never bypasses per-case policy checks: resolutions fan
// out through the existing per-case action path so PolicyGuard evaluates every
// case separately.
package assistant

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config controls grouping behaviour.
type Config struct {
	// MinCases is the minimum number of cases that must share a root cause
	// before they are grouped into one incident. Below the threshold the
	// cases stay standalone in the triage inbox.
	MinCases int
	// Window is how far apart two cases may be opened and still be treated
	// as part of the same incident.
	Window time.Duration
	// AutoSend allows low-risk status-update drafts to be sent without a
	// merchant approval. It is off by default: every send is an explicit
	// merchant action unless the deployment opts in.
	AutoSend bool
}

// DefaultConfig returns the quality-gate defaults.
func DefaultConfig() Config {
	return Config{MinCases: 2, Window: 6 * time.Hour, AutoSend: false}
}

func (c Config) withDefaults() Config {
	if c.MinCases <= 0 {
		c.MinCases = 2
	}
	if c.Window <= 0 {
		c.Window = 6 * time.Hour
	}
	return c
}

// ConfigFromEnv reads the assistant configuration from the environment.
func ConfigFromEnv() Config {
	cfg := DefaultConfig()
	if raw := os.Getenv("FRACTAL_INCIDENT_MIN_CASES"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			cfg.MinCases = n
		}
	}
	if raw := os.Getenv("FRACTAL_INCIDENT_WINDOW"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			cfg.Window = d
		}
	}
	switch os.Getenv("FRACTAL_ASSISTANT_AUTOSEND") {
	case "true", "1", "yes":
		cfg.AutoSend = true
	}
	return cfg
}

// Service is the assistant entry point.
type Service struct {
	db  *pgxpool.Pool
	log *slog.Logger
	cfg Config
}

// New builds the assistant service.
func New(db *pgxpool.Pool, log *slog.Logger, cfg Config) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{db: db, log: log, cfg: cfg.withDefaults()}
}

// Config returns the effective configuration.
func (s *Service) Config() Config { return s.cfg }

// hashEvent builds the tamper-evident hash stored with an audit event.
func hashEvent(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
