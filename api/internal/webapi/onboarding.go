package webapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"fractal/api/internal/auth"
	"fractal/api/internal/provider"
	"fractal/api/internal/region"
)

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

const (
	maxClientIDLength     = 512
	maxClientSecretLength = 512
	maxPasswordLength     = 72 // bcrypt input limit
)

type merchantSummary struct {
	ID               string `json:"id"`
	LegalName        string `json:"legal_name"`
	Country          string `json:"country"`
	Currency         string `json:"currency"`
	OnboardingStatus string `json:"onboarding_status"`
}

type providerAccountSummary struct {
	ID                  string     `json:"id"`
	Provider            string     `json:"provider"`
	Environment         string     `json:"environment"`
	ClientID            string     `json:"client_id,omitempty"`
	WebhookID           string     `json:"webhook_id,omitempty"`
	WebhookURL          string     `json:"webhook_url,omitempty"`
	WebhookStatus       string     `json:"webhook_status"`
	Status              string     `json:"status"`
	WebhookRegisteredAt *time.Time `json:"webhook_registered_at,omitempty"`
	LastCheckedAt       *time.Time `json:"last_checked_at,omitempty"`
	WebhookError        string     `json:"webhook_error,omitempty"`
}

type onboardingStatus struct {
	Merchant        merchantSummary         `json:"merchant"`
	ProviderAccount *providerAccountSummary `json:"provider_account"`
	Ready           bool                    `json:"ready"`
	NextStep        string                  `json:"next_step"`
}

// Regions exposes the supported countries and their settlement currency.
// It is public because the signup screen needs it before an account exists.
func (h *Handlers) Regions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"regions": region.All(),
	})
}

type signupRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	LegalName   string `json:"legal_name"`
	Country     string `json:"country"`
	Currency    string `json:"currency"`
}

// Signup creates a merchant account and its first merchant admin in one step.
// PayPal credentials are collected separately so the account exists even if the
// merchant needs to come back with their PayPal app details.
func (h *Handlers) Signup(w http.ResponseWriter, r *http.Request) {
	var req signupRequest
	if err := decodeJSON(r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request", "Expected the signup details as JSON.")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !emailPattern.MatchString(email) {
		writeProblem(w, http.StatusBadRequest, "Invalid email", "Enter a valid email address.")
		return
	}
	password := req.Password
	if len(password) < 8 {
		writeProblem(w, http.StatusBadRequest, "Password too short", "Use at least 8 characters.")
		return
	}
	if len(password) > maxPasswordLength {
		writeProblem(w, http.StatusBadRequest, "Password too long", "Use 72 characters or fewer.")
		return
	}
	legalName := strings.TrimSpace(req.LegalName)
	if legalName == "" {
		writeProblem(w, http.StatusBadRequest, "Business name required", "Enter the legal name of your business.")
		return
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = legalName
	}

	country, ok := region.Resolve(req.Country)
	if !ok {
		writeProblem(w, http.StatusBadRequest, "Region required",
			"Choose the country where your business is registered. Region determines the settlement currency.")
		return
	}
	currency := strings.ToUpper(strings.TrimSpace(req.Currency))
	if currency == "" {
		currency = country.Currency
	}
	if !region.Allows(country, currency) {
		writeProblem(w, http.StatusBadRequest, "Currency not supported for this region",
			fmt.Sprintf("%s settles in %s. PayPal does not support %s for this region.", country.Name, strings.Join(country.Currencies, ", "), currency))
		return
	}

	if h.Secrets == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Onboarding unavailable", "Credential encryption is not configured.")
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Could not use that password", err.Error())
		return
	}

	tx, err := h.DB.Begin(r.Context())
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not create account", "The database transaction could not start.")
		return
	}
	defer tx.Rollback(r.Context())

	var userID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO users (email, display_name, kind, password_hash)
		VALUES ($1, $2, 'MERCHANT_USER', $3)
		RETURNING id::text`, email, displayName, hash).Scan(&userID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			writeProblem(w, http.StatusConflict, "Account already exists",
				"That email already has a FRACTAL account. Sign in instead.")
			return
		}
		h.Log.Error("create onboarding user", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not create account", "The account could not be stored.")
		return
	}

	var merchant merchantSummary
	err = tx.QueryRow(r.Context(), `
		INSERT INTO merchants
		  (legal_name, country, currency, owner_user_id, onboarding_status,
		   integration_base_url, integration_key_ref)
		VALUES ($1, $2, $3, $4, 'PENDING_PAYPAL', '', '')
		RETURNING id::text, legal_name, trim(country), trim(currency), onboarding_status`,
		legalName, country.Code, currency, userID).
		Scan(&merchant.ID, &merchant.LegalName, &merchant.Country, &merchant.Currency, &merchant.OnboardingStatus)
	if err != nil {
		h.Log.Error("create onboarding merchant", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not create account", "The merchant profile could not be stored.")
		return
	}

	if _, err := tx.Exec(r.Context(), `
		INSERT INTO merchant_members (merchant_id, user_id, role, permissions)
		VALUES ($1, $2, 'MERCHANT_ADMIN', ARRAY['reconcile','refund'])`,
		merchant.ID, userID); err != nil {
		h.Log.Error("create onboarding membership", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not create account", "The merchant membership could not be stored.")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not create account", "The account could not be saved.")
		return
	}

	token, err := auth.Sign(auth.Claims{
		Sub:         userID,
		Kind:        "MERCHANT_USER",
		MerchantID:  merchant.ID,
		Role:        "MERCHANT_ADMIN",
		Permissions: []string{"reconcile", "refund"},
	})
	if err != nil {
		h.Log.Error("sign signup token", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Account created", "Sign in to continue.")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"token": token,
		"user": loginUser{
			Sub:          userID,
			Kind:         "MERCHANT_USER",
			MerchantID:   merchant.ID,
			MerchantName: merchant.LegalName,
			Role:         "MERCHANT_ADMIN",
			Permissions:  []string{"reconcile", "refund"},
			Email:        email,
			DisplayName:  displayName,
		},
		"merchant":  merchant,
		"next_step": "connect_paypal",
	})
}

type connectPayPalRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Environment  string `json:"environment"`
}

// ConnectPayPal stores encrypted credentials, registers this merchant's webhook
// URL with PayPal, and records the returned webhook ID.
func (h *Handlers) ConnectPayPal(w http.ResponseWriter, r *http.Request) {
	_, merchantID, ok := h.requireMerchantAdmin(w, r)
	if !ok {
		return
	}

	var req connectPayPalRequest
	if err := decodeJSON(r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request", "Expected PayPal credentials as JSON.")
		return
	}

	clientID := strings.TrimSpace(req.ClientID)
	clientSecret := strings.TrimSpace(req.ClientSecret)
	if clientID == "" || clientSecret == "" {
		writeProblem(w, http.StatusBadRequest, "PayPal credentials required",
			"Paste both the Client ID and the Secret from your PayPal app.")
		return
	}
	if len(clientID) > maxClientIDLength || len(clientSecret) > maxClientSecretLength {
		writeProblem(w, http.StatusBadRequest, "PayPal credentials too long", "Check that you pasted only the app credentials.")
		return
	}
	environment := strings.ToUpper(strings.TrimSpace(req.Environment))
	if environment == "" {
		environment = "SANDBOX"
	}
	if environment != "SANDBOX" && environment != "LIVE" {
		writeProblem(w, http.StatusBadRequest, "Invalid environment", "Choose sandbox or live.")
		return
	}
	if environment == "LIVE" {
		writeProblem(w, http.StatusForbidden, "Live PayPal is not enabled",
			"This hackathon build accepts PayPal sandbox credentials only.")
		return
	}

	var country, currency string
	if err := h.DB.QueryRow(r.Context(), `
		SELECT trim(country), trim(currency) FROM merchants WHERE id = $1`,
		merchantID).Scan(&country, &currency); err != nil {
		writeProblem(w, http.StatusNotFound, "Merchant not found", "This merchant account no longer exists.")
		return
	}
	if country == "" || currency == "" {
		writeProblem(w, http.StatusBadRequest, "Region missing", "Choose your region before connecting PayPal.")
		return
	}

	// PayPal rejects localhost and plain-HTTP webhook URLs. Check the origin
	// before storing an account so the error is actionable and no half-created
	// provider account is left behind.
	if _, err := h.publicBaseURL(r); err != nil {
		writeProblem(w, http.StatusBadRequest, "Public webhook URL required", err.Error())
		return
	}

	if h.Secrets == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Onboarding unavailable", "Credential encryption is not configured.")
		return
	}
	sealed, err := h.Secrets.Encrypt(clientSecret)
	if err != nil {
		h.Log.Error("encrypt paypal secret", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not store credentials", "The secret could not be encrypted.")
		return
	}

	// Reconnecting an existing merchant keeps the same provider account ID, so
	// the webhook URL PayPal already knows does not change.
	var accountID string
	err = h.DB.QueryRow(r.Context(), `
		SELECT id::text FROM provider_accounts
		WHERE merchant_id = $1 AND provider = 'PAYPAL' AND environment = $2
		ORDER BY created_at ASC LIMIT 1`, merchantID, environment).Scan(&accountID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		err = h.DB.QueryRow(r.Context(), `
			INSERT INTO provider_accounts
			  (merchant_id, provider, environment, external_account_id, credentials_ref,
			   client_id, encrypted_secret, status, webhook_status)
			VALUES ($1, 'PAYPAL', $2, $3, 'encrypted', $3, $4, 'INACTIVE', 'REGISTERING')
			RETURNING id::text`,
			merchantID, environment, clientID, sealed).Scan(&accountID)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeProblem(w, http.StatusConflict, "PayPal app already connected",
					"These PayPal credentials are already linked to another FRACTAL merchant.")
				return
			}
			h.Log.Error("insert provider account", "err", err)
			writeProblem(w, http.StatusInternalServerError, "Could not connect PayPal", "The provider account could not be stored.")
			return
		}
	case err != nil:
		h.Log.Error("lookup provider account", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not connect PayPal", "The provider account could not be loaded.")
		return
	default:
		if _, err := h.DB.Exec(r.Context(), `
			UPDATE provider_accounts
			SET client_id = $2, encrypted_secret = $3, external_account_id = $2,
			    status = 'INACTIVE', webhook_status = 'REGISTERING', webhook_error = NULL,
			    updated_at = now()
			WHERE id = $1`, accountID, clientID, sealed); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeProblem(w, http.StatusConflict, "PayPal app already connected",
					"These PayPal credentials are already linked to another FRACTAL merchant.")
				return
			}
			h.Log.Error("update provider account", "err", err)
			writeProblem(w, http.StatusInternalServerError, "Could not connect PayPal", "The provider account could not be updated.")
			return
		}
	}

	webhookURL, err := h.webhookURL(r, accountID)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Public webhook URL required", err.Error())
		return
	}
	account, err := h.registerWebhook(r.Context(), accountID, environment, clientID, clientSecret, webhookURL)
	if err != nil {
		h.Log.Warn("paypal webhook registration failed", "account", accountID, "url", webhookURL, "err", err)
		var apiErr *provider.APIError
		switch {
		case errors.As(err, &apiErr) && apiErr.Status == http.StatusUnauthorized:
			writeProblem(w, http.StatusBadGateway, "PayPal rejected the app credentials",
				"PayPal did not recognise this Client ID and Secret. Create a sandbox REST app in the PayPal Developer Dashboard and paste that app's credentials. "+safeError(err))
		case errors.As(err, &apiErr) && apiErr.Status == http.StatusBadRequest:
			writeProblem(w, http.StatusBadGateway, "PayPal rejected the webhook URL",
				"PayPal accepted the credentials but not the webhook address. It must be a public HTTPS URL. "+safeError(err))
		default:
			writeProblem(w, http.StatusBadGateway, "PayPal rejected the connection",
				"PayPal did not accept the credentials or the webhook registration. Check the app's sandbox credentials and try again. "+safeError(err))
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"provider_account": account,
		"merchant":         h.merchantSummary(r.Context(), merchantID),
	})
}

// ReRegisterWebhook re-runs webhook registration for an existing account. It is
// the recovery path when a webhook is deleted in PayPal or the public URL changes.
func (h *Handlers) ReRegisterWebhook(w http.ResponseWriter, r *http.Request) {
	_, merchantID, ok := h.requireMerchantAdmin(w, r)
	if !ok {
		return
	}
	if h.Secrets == nil {
		writeProblem(w, http.StatusServiceUnavailable, "Onboarding unavailable", "Credential encryption is not configured.")
		return
	}

	var (
		accountID   string
		environment string
		clientID    string
		sealed      string
		webhookURL  string
	)
	err := h.DB.QueryRow(r.Context(), `
		SELECT id::text, environment, COALESCE(client_id, ''), COALESCE(encrypted_secret, ''),
		       COALESCE(webhook_url, '')
		FROM provider_accounts
		WHERE merchant_id = $1 AND provider = 'PAYPAL'
		ORDER BY created_at ASC LIMIT 1`, merchantID).
		Scan(&accountID, &environment, &clientID, &sealed, &webhookURL)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "PayPal not connected", "Connect PayPal before registering a webhook.")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not load PayPal account", "The provider account could not be read.")
		return
	}
	if sealed == "" {
		writeProblem(w, http.StatusBadRequest, "Legacy credentials", "This account uses deployment credentials and cannot be re-registered here.")
		return
	}
	secret, err := h.Secrets.Decrypt(sealed)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "Could not read credentials", "Stored credentials could not be decrypted.")
		return
	}
	stored := strings.TrimRight(strings.TrimSpace(webhookURL), "/")
	derived, derr := h.webhookURL(r, accountID)
	switch {
	case derr == nil:
		// The configured origin wins, so a new tunnel/public URL is applied.
		webhookURL = derived
	case stored != "" && isPublicHTTPSURL(stored):
		// The merchant is re-registering from localhost but already has a
		// working public registration; keep it instead of downgrading it.
		webhookURL = stored
	default:
		writeProblem(w, http.StatusBadRequest, "Public webhook URL required", derr.Error())
		return
	}

	account, err := h.registerWebhook(r.Context(), accountID, environment, clientID, secret, webhookURL)
	if err != nil {
		h.Log.Warn("paypal webhook re-registration failed", "account", accountID, "err", err)
		writeProblem(w, http.StatusBadGateway, "Webhook registration failed", safeError(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider_account": account})
}

// OnboardingStatus returns the merchant, region and webhook state for the
// signed-in merchant. It never includes the stored secret.
func (h *Handlers) OnboardingStatus(w http.ResponseWriter, r *http.Request) {
	_, merchantID, ok := h.requireMerchantAdmin(w, r)
	if !ok {
		return
	}

	var out onboardingStatus
	err := h.DB.QueryRow(r.Context(), `
		SELECT id::text, legal_name, trim(country), trim(currency), onboarding_status
		FROM merchants WHERE id = $1`, merchantID).
		Scan(&out.Merchant.ID, &out.Merchant.LegalName, &out.Merchant.Country,
			&out.Merchant.Currency, &out.Merchant.OnboardingStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "Merchant not found", "This merchant account no longer exists.")
		return
	}
	if err != nil {
		h.Log.Error("load onboarding merchant", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not load onboarding", "The merchant could not be read.")
		return
	}

	account, err := h.providerAccount(r.Context(), merchantID)
	if err != nil {
		h.Log.Error("load onboarding provider account", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Could not load onboarding", "The provider account could not be read.")
		return
	}
	if account != nil {
		out.ProviderAccount = account
	}
	out.Ready = account != nil && account.WebhookStatus == "REGISTERED" && account.Status == "ACTIVE"
	switch {
	case out.Ready:
		out.NextStep = "done"
	case account != nil:
		out.NextStep = "retry_webhook"
	default:
		out.NextStep = "connect_paypal"
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handlers) providerAccount(ctx context.Context, merchantID string) (*providerAccountSummary, error) {
	var (
		account   providerAccountSummary
		webhookID sql.NullString
		regAt     sql.NullTime
		checkedAt sql.NullTime
	)
	err := h.DB.QueryRow(ctx, `
		SELECT id::text, provider, environment, COALESCE(client_id, ''), webhook_id,
		       COALESCE(webhook_url, ''), webhook_status, status, webhook_registered_at,
		       last_checked_at, COALESCE(webhook_error, '')
		FROM provider_accounts
		WHERE merchant_id = $1 AND provider = 'PAYPAL'
		ORDER BY created_at ASC LIMIT 1`, merchantID).
		Scan(&account.ID, &account.Provider, &account.Environment, &account.ClientID,
			&webhookID, &account.WebhookURL, &account.WebhookStatus, &account.Status,
			&regAt, &checkedAt, &account.WebhookError)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if webhookID.Valid {
		account.WebhookID = webhookID.String
	}
	if regAt.Valid {
		account.WebhookRegisteredAt = &regAt.Time
	}
	if checkedAt.Valid {
		account.LastCheckedAt = &checkedAt.Time
	}
	return &account, nil
}

func (h *Handlers) merchantSummary(ctx context.Context, merchantID string) merchantSummary {
	var summary merchantSummary
	_ = h.DB.QueryRow(ctx, `
		SELECT id::text, legal_name, trim(country), trim(currency), onboarding_status
		FROM merchants WHERE id = $1`, merchantID).
		Scan(&summary.ID, &summary.LegalName, &summary.Country, &summary.Currency, &summary.OnboardingStatus)
	return summary
}

func (h *Handlers) registerWebhook(ctx context.Context, accountID, environment, clientID, clientSecret, webhookURL string) (*providerAccountSummary, error) {
	registerCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	client := provider.NewPayPal(provider.BaseURLForEnvironment(environment), clientID, clientSecret, h.Log)
	webhook, err := client.EnsureWebhook(registerCtx, webhookURL)
	if err != nil {
		safe := truncate(safeError(err), 500)
		_, _ = h.DB.Exec(context.WithoutCancel(ctx), `
			UPDATE provider_accounts
			SET webhook_status = 'FAILED', status = 'INACTIVE', webhook_error = $2,
			    webhook_url = $3, last_checked_at = now(), updated_at = now()
			WHERE id = $1`, accountID, safe, webhookURL)
		return nil, err
	}

	var (
		account   providerAccountSummary
		regAt     sql.NullTime
		checkedAt sql.NullTime
		webhookID sql.NullString
	)
	err = h.DB.QueryRow(ctx, `
		UPDATE provider_accounts
		SET webhook_id = $2, webhook_url = $3, webhook_status = 'REGISTERED',
		    webhook_registered_at = now(), webhook_error = NULL, last_checked_at = now(),
		    status = 'ACTIVE', updated_at = now()
		WHERE id = $1
		RETURNING id::text, provider, environment, COALESCE(client_id, ''), webhook_id,
		          COALESCE(webhook_url, ''), webhook_status, status, webhook_registered_at,
		          last_checked_at, COALESCE(webhook_error, '')`,
		accountID, webhook.ID, webhookURL).
		Scan(&account.ID, &account.Provider, &account.Environment, &account.ClientID,
			&webhookID, &account.WebhookURL, &account.WebhookStatus, &account.Status,
			&regAt, &checkedAt, &account.WebhookError)
	if err != nil {
		return nil, fmt.Errorf("store webhook registration: %w", err)
	}
	if webhookID.Valid {
		account.WebhookID = webhookID.String
	}
	if regAt.Valid {
		account.WebhookRegisteredAt = &regAt.Time
	}
	if checkedAt.Valid {
		account.LastCheckedAt = &checkedAt.Time
	}

	_, _ = h.DB.Exec(ctx, `
		UPDATE merchants SET onboarding_status = 'ACTIVE', updated_at = now() WHERE id = (
			SELECT merchant_id FROM provider_accounts WHERE id = $1
		)`, accountID)
	return &account, nil
}

func (h *Handlers) requireMerchantAdmin(w http.ResponseWriter, r *http.Request) (*auth.Claims, string, bool) {
	claims := claimsFrom(r)
	if claims == nil {
		writeProblem(w, http.StatusUnauthorized, "Unauthenticated", "Sign in to continue.")
		return nil, "", false
	}
	if claims.MerchantID == "" {
		writeProblem(w, http.StatusForbidden, "Merchant account required", "Sign in with a merchant account.")
		return nil, "", false
	}
	if claims.Kind != "MERCHANT_USER" && claims.Kind != "PLATFORM_ADMIN" {
		writeProblem(w, http.StatusForbidden, "Merchant account required", "Sign in with a merchant account.")
		return nil, "", false
	}
	if claims.Kind == "MERCHANT_USER" && claims.Role != "" && claims.Role != "MERCHANT_ADMIN" {
		writeProblem(w, http.StatusForbidden, "Admin required", "Ask a merchant admin to connect PayPal.")
		return nil, "", false
	}
	return claims, claims.MerchantID, true
}

// publicBaseURL returns the externally reachable origin used to build PayPal
// webhook URLs. PayPal only accepts public HTTPS URLs, so a localhost or
// plain-HTTP origin fails here with an actionable message instead of becoming
// an opaque "Invalid data" error from PayPal.
func (h *Handlers) publicBaseURL(r *http.Request) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(h.PublicBaseURL), "/")
	if base == "" {
		// Local development keeps the tunnel origin in PUBLIC_TUNNEL_URL.
		base = strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_TUNNEL_URL")), "/")
	}
	if base == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if proto := firstHeaderToken(r.Header.Get("X-Forwarded-Proto")); proto != "" {
			scheme = proto
		}
		host := firstHeaderToken(r.Header.Get("X-Forwarded-Host"))
		if host == "" {
			host = r.Host
		}
		if host != "" {
			base = scheme + "://" + host
		}
	}
	if base == "" {
		return "", errors.New("no public app URL is configured. Set FRACTAL_PUBLIC_URL to the app's HTTPS origin.")
	}
	return validatePublicBaseURL(base)
}

func validatePublicBaseURL(base string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("FRACTAL_PUBLIC_URL %q is not a valid absolute URL", base)
	}
	if path := strings.Trim(parsed.EscapedPath(), "/"); path != "" {
		return "", errors.New("FRACTAL_PUBLIC_URL must be an origin such as https://fractal.example.com, without a path")
	}
	if err := validatePublicHTTPS(parsed); err != nil {
		return "", err
	}
	return strings.TrimRight(base, "/"), nil
}

func validatePublicHTTPS(parsed *url.URL) error {
	if !strings.EqualFold(parsed.Scheme, "https") {
		return fmt.Errorf("PayPal requires an HTTPS webhook URL, but the app origin is %s. Set FRACTAL_PUBLIC_URL to the app's public HTTPS origin; for local testing use your tunnel URL.", parsed.String())
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0":
		return fmt.Errorf("PayPal cannot deliver webhooks to %s. Set FRACTAL_PUBLIC_URL to the app's public HTTPS origin; for local testing use your tunnel URL.", parsed.String())
	}
	return nil
}

func isPublicHTTPSURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return false
	}
	return validatePublicHTTPS(parsed) == nil
}

func firstHeaderToken(value string) string {
	if i := strings.IndexByte(value, ','); i >= 0 {
		value = value[:i]
	}
	return strings.TrimSpace(value)
}

func (h *Handlers) webhookURL(r *http.Request, accountID string) (string, error) {
	base, err := h.publicBaseURL(r)
	if err != nil {
		return "", err
	}
	return base + "/webhooks/paypal/" + accountID, nil
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	return truncate(err.Error(), 500)
}

func truncate(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
