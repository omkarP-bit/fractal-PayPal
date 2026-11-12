package webapi

import (
	"database/sql"
	"net/http"
	"strings"

	"fractal/api/internal/auth"
)

type devLoginRequest struct {
	Role  string `json:"role"`
	Email string `json:"email"`
}

type passwordLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// PasswordLogin checks the bcrypt hash created at merchant signup. Seeded demo
// accounts have no password hash and continue to use the dev-login endpoint.
func (h *Handlers) PasswordLogin(w http.ResponseWriter, r *http.Request) {
	var req passwordLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request", "Expected an email and password.")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || req.Password == "" {
		writeProblem(w, http.StatusBadRequest, "Email and password required", "Enter both your email and password.")
		return
	}

	var (
		id           string
		userEmail    string
		displayName  string
		kind         string
		passwordHash sql.NullString
		merchantID   string
		merchantName string
		memberRole   string
		permissions  []string
	)
	err := h.DB.QueryRow(r.Context(), `
		SELECT u.id::text, u.email::text, u.display_name, u.kind, u.password_hash,
		       COALESCE(mm.merchant_id::text, ''), COALESCE(m.legal_name, ''),
		       COALESCE(mm.role, ''), COALESCE(mm.permissions, '{}')
		FROM users u
		LEFT JOIN merchant_members mm ON mm.user_id = u.id
		LEFT JOIN merchants m ON m.id = mm.merchant_id
		WHERE u.email = $1 AND u.status = 'ACTIVE'
		ORDER BY (mm.merchant_id IS NOT NULL) DESC
		LIMIT 1`, email).Scan(
		&id, &userEmail, &displayName, &kind, &passwordHash,
		&merchantID, &merchantName, &memberRole, &permissions,
	)
	if err != nil || !passwordHash.Valid {
		writeProblem(w, http.StatusUnauthorized, "Sign-in failed",
			"That email and password do not match an active account.")
		return
	}
	if !auth.CheckPassword(passwordHash.String, req.Password) {
		writeProblem(w, http.StatusUnauthorized, "Sign-in failed",
			"That email and password do not match an active account.")
		return
	}

	if kind == "MERCHANT_USER" && merchantID == "" {
		writeProblem(w, http.StatusForbidden, "No merchant membership",
			"This merchant user is not attached to a store.")
		return
	}
	if kind == "CUSTOMER" {
		merchantID = ""
		merchantName = ""
		memberRole = ""
		permissions = []string{}
	}

	token, err := auth.Sign(auth.Claims{
		Sub:         id,
		Kind:        kind,
		MerchantID:  merchantID,
		Role:        memberRole,
		Permissions: permissions,
	})
	if err != nil {
		h.Log.Error("sign password token", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Sign in failed", "Could not sign a session token.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user": loginUser{
			Sub:          id,
			Kind:         kind,
			MerchantID:   merchantID,
			MerchantName: merchantName,
			Role:         memberRole,
			Permissions:  permissions,
			Email:        userEmail,
			DisplayName:  displayName,
		},
	})
}

type loginUser struct {
	Sub          string   `json:"sub"`
	Kind         string   `json:"kind"`
	MerchantID   string   `json:"merchant_id,omitempty"`
	MerchantName string   `json:"merchant_name,omitempty"`
	Role         string   `json:"role,omitempty"`
	Permissions  []string `json:"permissions,omitempty"`
	Email        string   `json:"email"`
	DisplayName  string   `json:"display_name"`
}

// DevLogin is a development-only credential exchange. It reads a seeded user
// and issues the same HS256 JWT the middleware verifies. No password is
// checked, and this endpoint is intentionally not part of a production build.
func (h *Handlers) DevLogin(w http.ResponseWriter, r *http.Request) {
	var req devLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request", "Expected JSON with a role and optional email.")
		return
	}

	role := strings.ToUpper(strings.TrimSpace(req.Role))
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if role == "" {
		role = "CUSTOMER"
	}
	if email == "" {
		writeProblem(w, http.StatusBadRequest, "Email required",
			"Enter the email address attached to your account.")
		return
	}

	var (
		id           string
		userEmail    string
		displayName  string
		kind         string
		passwordHash sql.NullString
		merchantID   string
		merchantName string
		memberRole   string
		permissions  []string
	)
	err := h.DB.QueryRow(r.Context(), `
		SELECT u.id::text, u.email::text, u.display_name, u.kind, u.password_hash,
		       COALESCE(mm.merchant_id::text, ''), COALESCE(m.legal_name, ''),
		       COALESCE(mm.role, ''), COALESCE(mm.permissions, '{}')
		FROM users u
		LEFT JOIN merchant_members mm ON mm.user_id = u.id
		LEFT JOIN merchants m ON m.id = mm.merchant_id
		WHERE u.email = $1 AND u.status = 'ACTIVE'
		ORDER BY (mm.merchant_id IS NOT NULL) DESC
		LIMIT 1`, email).Scan(
		&id, &userEmail, &displayName, &kind, &passwordHash,
		&merchantID, &merchantName, &memberRole, &permissions,
	)
	if err != nil {
		writeProblem(w, http.StatusUnauthorized,
			"Account not found",
			"No active account matches that email address.")
		return
	}
	// Self-service accounts must use their password. The dev shortcut is only
	// for seeded demo users that have no password hash.
	if passwordHash.Valid {
		writeProblem(w, http.StatusUnauthorized,
			"Password required",
			"This account uses password sign-in. Enter your password.")
		return
	}

	if role == "MERCHANT" {
		if kind != "MERCHANT_USER" && kind != "PLATFORM_ADMIN" {
			writeProblem(w, http.StatusForbidden, "Not a merchant account",
				"This user is not a member of a merchant team.")
			return
		}
		if merchantID == "" {
			writeProblem(w, http.StatusForbidden, "No merchant membership",
				"This merchant user is not attached to a store.")
			return
		}
	} else {
		kind = "CUSTOMER"
		merchantID = ""
		merchantName = ""
		memberRole = ""
		permissions = []string{}
	}

	token, err := auth.Sign(auth.Claims{
		Sub:         id,
		Kind:        kind,
		MerchantID:  merchantID,
		Role:        memberRole,
		Permissions: permissions,
	})
	if err != nil {
		h.Log.Error("sign dev token", "err", err)
		writeProblem(w, http.StatusInternalServerError, "Sign in failed", "Could not sign a session token.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user": loginUser{
			Sub:          id,
			Kind:         kind,
			MerchantID:   merchantID,
			MerchantName: merchantName,
			Role:         memberRole,
			Permissions:  permissions,
			Email:        userEmail,
			DisplayName:  displayName,
		},
	})
}
