package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/XferOps/hizal/internal/auth"
	"github.com/XferOps/hizal/internal/audit"
	"github.com/XferOps/hizal/internal/models"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandlers struct {
	pool        *pgxpool.Pool
	auditLogger *audit.AuditLogger
}

const (
	minPasswordLength = 8
	maxPasswordLength = 128
)

type passwordValidationError struct {
	message string
}

func (e *passwordValidationError) Error() string {
	return e.message
}

func NewAuthHandlers(pool *pgxpool.Pool, auditLogger *audit.AuditLogger) *AuthHandlers {
	return &AuthHandlers{pool: pool, auditLogger: auditLogger}
}

func getClientIP(r *http.Request) string {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ip = xff
	}
	return ip
}

func validatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < minPasswordLength || length > maxPasswordLength {
		return &passwordValidationError{
			message: fmt.Sprintf("password must be between %d and %d characters", minPasswordLength, maxPasswordLength),
		}
	}
	return nil
}

func writePasswordValidationError(w http.ResponseWriter, err error) bool {
	var validationErr *passwordValidationError
	if !errors.As(err, &validationErr) {
		return false
	}

	writeError(w, http.StatusBadRequest, "INVALID_PASSWORD", validationErr.Error())
	return true
}

// registerUser creates a user record and returns the new user ID + JWT.
// Extracted so invite_handlers can reuse the logic without going through HTTP.
func (h *AuthHandlers) registerUser(ctx context.Context, email, password, name string) (userID, token string, err error) {
	if err := validatePassword(password); err != nil {
		return "", "", err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}
	err = h.pool.QueryRow(ctx, `
		INSERT INTO users (email, name, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id
	`, email, name, string(hash)).Scan(&userID)
	if err != nil {
		return "", "", err
	}
	token, err = SignJWT(userID, email)
	return userID, token, err
}

// POST /v1/auth/register
//
// Auto-provisions a personal workspace on signup:
// 1. Creates user
// 2. Creates a personal org ("{name}'s Workspace")
// 3. Creates org membership (owner)
// 4. Creates a default project ("My Project")
// 5. Generates a default API key scoped to that project
//
// Returns everything the user needs to start using Hizal immediately.
func (h *AuthHandlers) Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeJSONDecodeError(w, err, "")
		return
	}
	if body.Email == "" || body.Password == "" || body.Name == "" {
		writeError(w, http.StatusBadRequest, "MISSING_FIELDS", "email, password, and name are required")
		return
	}
	if err := validatePassword(body.Password); err != nil {
		writePasswordValidationError(w, err)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeInternalError(r, w, "HASH_FAILED", err)
		return
	}

	// Everything in one transaction — if anything fails, nothing is created.
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}
	defer tx.Rollback(r.Context())

	// 1. Create user
	var user models.User
	err = tx.QueryRow(r.Context(), `
		INSERT INTO users (email, name, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, name
	`, body.Email, body.Name, string(hash)).Scan(&user.ID, &user.Email, &user.Name)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "EMAIL_TAKEN", "a user with that email already exists")
			return
		}
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	// 2. Create personal org
	orgName := fmt.Sprintf("%s's Workspace", body.Name)
	orgSlug := deriveOrgSlug(body.Email)

	var orgID, finalOrgSlug string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO orgs (name, slug, is_personal) VALUES ($1, $2, TRUE) RETURNING id, slug
	`, orgName, orgSlug).Scan(&orgID, &finalOrgSlug)
	if err != nil {
		if isUniqueViolation(err) {
			// Slug collision — append random suffix and retry
			orgSlug = orgSlug + "-" + randomSuffix()
			err = tx.QueryRow(r.Context(), `
				INSERT INTO orgs (name, slug, is_personal) VALUES ($1, $2, TRUE) RETURNING id, slug
			`, orgName, orgSlug).Scan(&orgID, &finalOrgSlug)
			if err != nil {
				writeInternalError(r, w, "DB_ERROR", err)
				return
			}
		} else {
			writeInternalError(r, w, "DB_ERROR", err)
			return
		}
	}

	// 3. Create org membership (owner)
	_, err = tx.Exec(r.Context(), `
		INSERT INTO org_memberships (user_id, org_id, role) VALUES ($1, $2, 'owner')
	`, user.ID, orgID)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	// 4. Create default project
	var projectID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO projects (org_id, name, slug, description)
		VALUES ($1, 'My Project', 'my-project', 'Your first Hizal project. Connect a repo to get started.')
		RETURNING id
	`, orgID).Scan(&projectID)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	// 5. Generate default API key scoped to the project
	plaintext, keyHash, err := auth.GenerateAPIKey(finalOrgSlug)
	if err != nil {
		writeInternalError(r, w, "KEYGEN_FAILED", err)
		return
	}

	var keyID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO api_keys (owner_type, user_id, org_id, key_hash, name, scope_all_projects, allowed_project_ids)
		VALUES ('USER', $1, $2, $3, 'Default Key', false, ARRAY[$4]::uuid[])
		RETURNING id
	`, user.ID, orgID, keyHash, projectID).Scan(&keyID)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	// Commit the transaction
	if err := tx.Commit(r.Context()); err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	token, err := SignJWT(user.ID, user.Email)
	if err != nil {
		writeInternalError(r, w, "JWT_FAILED", err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"token":   token,
		"user_id": user.ID,
		"email":   user.Email,
		"name":    user.Name,
		"setup": map[string]interface{}{
			"org": map[string]interface{}{
				"id":   orgID,
				"name": orgName,
				"slug": finalOrgSlug,
			},
			"project": map[string]interface{}{
				"id":   projectID,
				"name": "My Project",
				"slug": "my-project",
			},
			"api_key": map[string]interface{}{
				"id":   keyID,
				"key":  plaintext,
				"name": "Default Key",
				"note": "Store this key securely — it will not be shown again.",
			},
		},
	})
}

var slugSanitizer = regexp.MustCompile(`[^a-z0-9-]+`)

// deriveOrgSlug creates a URL-safe slug from the email prefix.
// "jane.smith@example.com" → "jane-smith"
func deriveOrgSlug(email string) string {
	parts := strings.SplitN(email, "@", 2)
	slug := strings.ToLower(parts[0])
	slug = slugSanitizer.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "workspace"
	}
	// Cap length
	if len(slug) > 40 {
		slug = slug[:40]
	}
	return slug
}

// randomSuffix returns a short random hex string for deduplication.
func randomSuffix() string {
	b := make([]byte, 3)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// POST /v1/auth/login
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeJSONDecodeError(w, err, "")
		return
	}

	ip := getClientIP(r)
	userAgent := r.Header.Get("User-Agent")

	var user models.User
	var hash string
	err := h.pool.QueryRow(r.Context(), `
		SELECT id, email, name, COALESCE(password_hash, '') FROM users WHERE email = $1
	`, body.Email).Scan(&user.ID, &user.Email, &user.Name, &hash)
	if err != nil || hash == "" {
		if h.auditLogger != nil {
			h.auditLogger.Log(r.Context(), audit.Entry{
				OrgID:      "",
				ActorType:  audit.ActorTypeUser,
				ActorID:    "",
				Action:     "LOGIN_FAILED",
				Metadata:   map[string]any{"email": body.Email, "reason": "user_not_found"},
				IP:         &ip,
				UserAgent:  &userAgent,
			})
		}
		writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)); err != nil {
		if h.auditLogger != nil {
			h.auditLogger.Log(r.Context(), audit.Entry{
				OrgID:      "",
				ActorType:  audit.ActorTypeUser,
				ActorID:    user.ID,
				ActorEmail: &user.Email,
				Action:     "LOGIN_FAILED",
				Metadata:   map[string]any{"reason": "invalid_password"},
				IP:         &ip,
				UserAgent:  &userAgent,
			})
		}
		writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "invalid email or password")
		return
	}

	token, err := SignJWT(user.ID, user.Email)
	if err != nil {
		writeInternalError(r, w, "JWT_FAILED", err)
		return
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		writeInternalError(r, w, "TOKEN_GEN_FAILED", err)
		return
	}

	refreshHash := HashRefreshToken(refreshToken)
	expiresAt := time.Now().Add(7 * 24 * time.Hour)
	_, err = h.pool.Exec(r.Context(), `
		INSERT INTO refresh_tokens (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)
	`, refreshHash, user.ID, expiresAt)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	if h.auditLogger != nil {
		h.auditLogger.Log(r.Context(), audit.Entry{
			OrgID:      "",
			ActorType:  audit.ActorTypeUser,
			ActorID:    user.ID,
			ActorEmail: &user.Email,
			Action:     "LOGIN_SUCCESS",
			IP:         &ip,
			UserAgent:  &userAgent,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":         token,
		"refresh_token": refreshToken,
		"user_id":       user.ID,
		"email":         user.Email,
		"name":          user.Name,
	})
}

// POST /v1/auth/refresh
func (h *AuthHandlers) Refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeJSONDecodeError(w, err, "")
		return
	}

	if body.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "MISSING_FIELDS", "refresh_token is required")
		return
	}

	refreshHash := HashRefreshToken(body.RefreshToken)

	var tokenID string
	var userID string
	var expiresAt time.Time
	var revokedAt *time.Time

	err := h.pool.QueryRow(r.Context(), `
		SELECT id, user_id, expires_at, revoked_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`, refreshHash).Scan(&tokenID, &userID, &expiresAt, &revokedAt)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "invalid refresh token")
		return
	}

	if revokedAt != nil {
		writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "refresh token has been revoked")
		return
	}

	if time.Now().After(expiresAt) {
		writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "refresh token has expired")
		return
	}

	var email string
	err = h.pool.QueryRow(r.Context(), `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}
	defer tx.Rollback(r.Context())

	_, err = tx.Exec(r.Context(), `
		UPDATE refresh_tokens
		SET revoked_at = NOW()
		WHERE id = $1
	`, tokenID)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	newRefreshToken, err := GenerateRefreshToken()
	if err != nil {
		writeInternalError(r, w, "TOKEN_GEN_FAILED", err)
		return
	}

	newRefreshHash := HashRefreshToken(newRefreshToken)
	newExpiresAt := time.Now().Add(7 * 24 * time.Hour)

	_, err = tx.Exec(r.Context(), `
		INSERT INTO refresh_tokens (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)
	`, newRefreshHash, userID, newExpiresAt)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	newAccessToken, err := SignJWT(userID, email)
	if err != nil {
		writeInternalError(r, w, "JWT_FAILED", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":         newAccessToken,
		"refresh_token": newRefreshToken,
	})
}

// PATCH /v1/auth/me
func (h *AuthHandlers) UpdateUser(w http.ResponseWriter, r *http.Request) {
	user, ok := JWTUserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "not authenticated")
		return
	}

	var body struct {
		Name *string `json:"name"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeJSONDecodeError(w, err, "invalid request body")
		return
	}
	if body.Name == nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "name is required")
		return
	}
	if *body.Name == "" {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "name cannot be empty")
		return
	}

	var updatedUser models.User
	err := h.pool.QueryRow(r.Context(), `
		UPDATE users
		SET name = $1, updated_at = NOW()
		WHERE id = $2
		RETURNING id, email, name
	`, *body.Name, user.ID).Scan(&updatedUser.ID, &updatedUser.Email, &updatedUser.Name)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":    updatedUser.ID,
		"email": updatedUser.Email,
		"name":  updatedUser.Name,
	})
}

// GET /v1/auth/me
func (h *AuthHandlers) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := JWTUserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "not authenticated")
		return
	}

	var dbUser models.User
	err := h.pool.QueryRow(r.Context(), `SELECT id, email, name FROM users WHERE id = $1`, user.ID).Scan(&dbUser.ID, &dbUser.Email, &dbUser.Name)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}

	// Fetch orgs
	rows, err := h.pool.Query(r.Context(), `
		SELECT o.id, o.name, o.slug, o.tier, o.is_personal, om.role
		FROM orgs o
		JOIN org_memberships om ON om.org_id = o.id
		WHERE om.user_id = $1
		ORDER BY o.created_at
	`, user.ID)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}
	defer rows.Close()

	type orgItem struct {
		ID         string `json:"id"`
		Name       string `json:"name"`
		Slug       string `json:"slug"`
		Tier       string `json:"tier"`
		IsPersonal bool   `json:"is_personal"`
		Role       string `json:"role"`
	}
	var orgs []orgItem
	var personalOrgID string
	var personalTier string
	for rows.Next() {
		var org models.Org
		var isPersonal bool
		var role string
		if err := rows.Scan(&org.ID, &org.Name, &org.Slug, &org.Tier, &isPersonal, &role); err != nil {
			continue
		}
		if isPersonal {
			personalOrgID = org.ID
			personalTier = org.Tier
		}
		orgs = append(orgs, orgItem{
			ID:         org.ID,
			Name:       org.Name,
			Slug:       org.Slug,
			Tier:       org.Tier,
			IsPersonal: isPersonal,
			Role:       role,
		})
	}
	if orgs == nil {
		orgs = []orgItem{}
	}

	// Count locked projects across personal org for downgrade modal
	var lockedProjectCount int
	if personalOrgID != "" {
		h.pool.QueryRow(r.Context(), `
			SELECT COUNT(*) FROM projects
			WHERE org_id = $1 AND locked_at IS NOT NULL
		`, personalOrgID).Scan(&lockedProjectCount)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                   dbUser.ID,
		"email":                dbUser.Email,
		"name":                 dbUser.Name,
		"orgs":                 orgs,
		"personal_org_id":      personalOrgID,
		"tier":                 personalTier,
		"locked_project_count": lockedProjectCount,
	})
}
