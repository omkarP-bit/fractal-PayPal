package webapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"fractal/api/internal/assistant"
	"fractal/api/internal/auth"
	"fractal/api/internal/evidence"
	"fractal/api/internal/investigator"
	"fractal/api/internal/policy"
	"fractal/api/internal/provider"
	"fractal/api/internal/secrets"
)

type Handlers struct {
	DB                *pgxpool.Pool
	MerchantStoreBase string
	HTTP              *http.Client
	Log               *slog.Logger

	Policy                    *policy.Client
	PayPal                    *provider.PayPal
	Collector                 *evidence.Collector
	Investigator              *investigator.Client
	AnomalyEnabled            bool
	MerchantIntegrationSecret string

	// Assistant is the AI Case Assistant: incident grouping, the triage
	// inbox and drafted replies. It has no provider credentials and cannot
	// execute actions.
	Assistant *assistant.Service

	// Onboarding: Secrets seals provider credentials, PublicBaseURL is the
	// externally reachable app origin (Render or a tunnel), and KibanaURL is the
	// live observability dashboard.
	Secrets       *secrets.Cipher
	PublicBaseURL string
	KibanaURL     string
}

func New(db *pgxpool.Pool, log *slog.Logger, policyClient *policy.Client, paypal *provider.PayPal, anomalyEnabled bool) *Handlers {
	base := strings.TrimRight(os.Getenv("MERCHANT_STORE_BASE"), "/")
	if base == "" {
		base = "http://localhost:8090"
	}
	collector := evidence.NewCollector(db, paypal, merchantAPI{
		base:   base,
		client: &http.Client{Timeout: 12 * time.Second},
	})
	h := &Handlers{
		DB:                        db,
		MerchantStoreBase:         base,
		HTTP:                      &http.Client{Timeout: 12 * time.Second},
		Log:                       log,
		Policy:                    policyClient,
		PayPal:                    paypal,
		Collector:                 collector,
		Investigator:              investigator.New(os.Getenv("INVESTIGATOR_URL"), os.Getenv("INVESTIGATOR_TOKEN")),
		AnomalyEnabled:            anomalyEnabled,
		MerchantIntegrationSecret: os.Getenv("MERCHANT_INTEGRATION_SECRET"),
		Assistant:                 assistant.New(db, log, assistant.ConfigFromEnv()),
	}
	// Resolve merchant credentials lazily: Secrets and PublicBaseURL are set by
	// main after construction, and the AI layer only ever sees evidence.
	collector.ProviderFor = h.providerFetcherForCase
	return h
}

type merchantAPI struct {
	base   string
	client *http.Client
}

func (m merchantAPI) GetOrder(ctx context.Context, ref string) (evidence.OrderSnapshot, error) {
	url := fmt.Sprintf("%s/api/orders/%s", m.base, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return evidence.OrderSnapshot{}, err
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return evidence.OrderSnapshot{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return evidence.OrderSnapshot{}, fmt.Errorf("merchant order API returned %d", resp.StatusCode)
	}
	var raw struct {
		Status      string `json:"status"`
		AmountMinor int64  `json:"amount_minor"`
		Currency    string `json:"currency"`
		UpdatedAt   string `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return evidence.OrderSnapshot{}, err
	}
	fetchedAt, err := time.Parse(time.RFC3339, raw.UpdatedAt)
	if err != nil {
		fetchedAt = time.Now().UTC()
	}
	return evidence.OrderSnapshot{
		Ref:         ref,
		Status:      raw.Status,
		AmountMinor: raw.AmountMinor,
		Currency:    raw.Currency,
		FetchedAt:   fetchedAt,
	}, nil
}

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":   "https://fractal.dev/problems/" + strings.ToLower(strings.ReplaceAll(title, " ", "-")),
		"title":  title,
		"status": status,
		"detail": detail,
	})
}

func claimsFrom(r *http.Request) *auth.Claims { return auth.FromContext(r) }

func (h *Handlers) policyRevision(ctx context.Context) string {
	var revision string
	err := h.DB.QueryRow(ctx, `
		SELECT revision FROM policy_bundles
		WHERE status = 'ACTIVE'
		ORDER BY activated_at DESC NULLS LAST
		LIMIT 1`).Scan(&revision)
	if err == nil && strings.TrimSpace(revision) != "" {
		return revision
	}
	if configured := strings.TrimSpace(os.Getenv("FRACTAL_POLICY_REVISION")); configured != "" {
		return configured
	}
	return "unversioned"
}

func isMerchant(c *auth.Claims) bool {
	return c != nil && (c.Kind == "MERCHANT_USER" || c.Kind == "PLATFORM_ADMIN")
}

func isCustomer(c *auth.Claims) bool { return c != nil && c.Kind == "CUSTOMER" }

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func paramsJSON(amountMinor int64, currency string) []byte {
	return []byte(fmt.Sprintf(`{"amount_minor":%d,"currency":%q}`, amountMinor, currency))
}

func uuidText(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) != 36 || strings.Count(value, "-") != 4 {
		return "", false
	}
	return value, true
}

func (h *Handlers) partyFor(ctx context.Context, caseID string, claims *auth.Claims) (merchantID, customerID string, ok bool, err error) {
	err = h.DB.QueryRow(ctx, `
		SELECT merchant_id::text, customer_id::text
		FROM exception_cases WHERE id = $1`, caseID).Scan(&merchantID, &customerID)
	if err != nil {
		return "", "", false, err
	}
	if isCustomer(claims) && claims.Sub == customerID {
		return merchantID, customerID, true, nil
	}
	if isMerchant(claims) && claims.MerchantID != "" && claims.MerchantID == merchantID {
		return merchantID, customerID, true, nil
	}
	if claims != nil && claims.Kind == "PLATFORM_ADMIN" && claims.MerchantID == "" {
		return merchantID, customerID, true, nil
	}
	return merchantID, customerID, false, nil
}
