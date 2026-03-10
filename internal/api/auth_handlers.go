package api

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandlers struct {
	pool *pgxpool.Pool
}

func NewAuthHandlers(pool *pgxpool.Pool) *AuthHandlers {
	return &AuthHandlers{pool: pool}
}

// POST /v1/auth/register
func (h *AuthHandlers) Register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Name     string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if body.Email == "" || body.Password == "" || body.Name == "" {
		writeError(w, http.StatusBadRequest, "MISSING_FIELDS", "email, password, and name are required")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "HASH_FAILED", err.Error())
		return
	}

	var userID string
	err = h.pool.QueryRow(r.Context(), `
		INSERT INTO users (email, name, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id
	`, body.Email, body.Name, string(hash)).Scan(&userID)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "EMAIL_TAKEN", "a user with that email already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	token, err := SignJWT(userID, body.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "JWT_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"token":   token,
		"user_id": userID,
		"email":   body.Email,
		"name":    body.Name,
	})
}

// POST /v1/auth/login
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	var userID, name, hash string
	err := h.pool.QueryRow(r.Context(), `
		SELECT id, name, COALESCE(password_hash, '') FROM users WHERE email = $1
	`, body.Email).Scan(&userID, &name, &hash)
	if err != nil || hash == "" {
		writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "invalid email or password")
		return
	}

	token, err := SignJWT(userID, body.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "JWT_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":   token,
		"user_id": userID,
		"email":   body.Email,
		"name":    name,
	})
}

// GET /v1/auth/me
func (h *AuthHandlers) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := JWTUserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "not authenticated")
		return
	}

	var name string
	err := h.pool.QueryRow(r.Context(), `SELECT name FROM users WHERE id = $1`, user.ID).Scan(&name)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "user not found")
		return
	}

	// Fetch orgs
	rows, err := h.pool.Query(r.Context(), `
		SELECT o.id, o.name, o.slug, om.role
		FROM orgs o
		JOIN org_memberships om ON om.org_id = o.id
		WHERE om.user_id = $1
		ORDER BY o.created_at
	`, user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	defer rows.Close()

	type orgItem struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
		Role string `json:"role"`
	}
	var orgs []orgItem
	for rows.Next() {
		var o orgItem
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.Role); err != nil {
			continue
		}
		orgs = append(orgs, o)
	}
	if orgs == nil {
		orgs = []orgItem{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":    user.ID,
		"email": user.Email,
		"name":  name,
		"orgs":  orgs,
	})
}

// isUniqueViolation checks for Postgres unique constraint error (code 23505).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return containsStr(err.Error(), "23505") || containsStr(err.Error(), "unique")
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && stringContains(s, substr))
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
