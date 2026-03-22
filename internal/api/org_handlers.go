package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/XferOps/hizal/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OrgHandlers struct {
	pool *pgxpool.Pool
}

func NewOrgHandlers(pool *pgxpool.Pool) *OrgHandlers {
	return &OrgHandlers{pool: pool}
}

// requireOrgRole checks the current JWT user's role in an org and returns error if insufficient.
// Hierarchy: owner > admin > member > viewer
func requireOrgRole(r *http.Request, pool *pgxpool.Pool, orgID string, roles ...string) (string, error) {
	user, ok := JWTUserFrom(r.Context())
	if !ok {
		return "", errors.New("not authenticated")
	}
	var role string
	err := pool.QueryRow(r.Context(), `
		SELECT role FROM org_memberships WHERE user_id = $1 AND org_id = $2
	`, user.ID, orgID).Scan(&role)
	if err != nil {
		return "", errors.New("not a member of this org")
	}
	for _, allowed := range roles {
		if role == allowed {
			return role, nil
		}
	}
	return role, errors.New("insufficient permissions")
}

// POST /v1/orgs
func (h *OrgHandlers) CreateOrg(w http.ResponseWriter, r *http.Request) {
	user, ok := JWTUserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "not authenticated")
		return
	}
	var body struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" || body.Slug == "" {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "name and slug are required")
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	var org models.Org
	err = tx.QueryRow(r.Context(), `
		INSERT INTO orgs (name, slug) VALUES ($1, $2) RETURNING id, name, slug
	`, body.Name, body.Slug).Scan(&org.ID, &org.Name, &org.Slug)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "SLUG_TAKEN", "an org with that slug already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	_, err = tx.Exec(r.Context(), `
		INSERT INTO org_memberships (user_id, org_id, role) VALUES ($1, $2, 'owner')
	`, user.ID, org.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":   org.ID,
		"name": org.Name,
		"slug": org.Slug,
		"role": "owner",
	})
}

// GET /v1/orgs
func (h *OrgHandlers) ListOrgs(w http.ResponseWriter, r *http.Request) {
	user, ok := JWTUserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "not authenticated")
		return
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT
			o.id,
			o.name,
			o.slug,
			o.tier,
			o.created_at,
			om.role,
			(SELECT COUNT(*) FROM org_memberships om2 WHERE om2.org_id = o.id) AS member_count,
			(SELECT COUNT(*) FROM projects p WHERE p.org_id = o.id) AS project_count
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
		ID           string `json:"id"`
		Name         string `json:"name"`
		Slug         string `json:"slug"`
		Tier         string `json:"tier"`
		CreatedAt    string `json:"created_at"`
		Role         string `json:"role"`
		MemberCount  int64  `json:"member_count"`
		ProjectCount int64  `json:"project_count"`
	}
	var orgs []orgItem
	for rows.Next() {
		var org models.Org
		var item orgItem
		if err := rows.Scan(&org.ID, &org.Name, &org.Slug, &org.Tier, &org.CreatedAt, &item.Role, &item.MemberCount, &item.ProjectCount); err != nil {
			continue
		}
		item.ID = org.ID
		item.Name = org.Name
		item.Slug = org.Slug
		item.Tier = org.Tier
		item.CreatedAt = org.CreatedAt.Format(time.RFC3339)
		orgs = append(orgs, item)
	}
	if orgs == nil {
		orgs = []orgItem{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"orgs": orgs})
}

// GET /v1/orgs/:id
func (h *OrgHandlers) GetOrg(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	user, ok := JWTUserFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "AUTH_REQUIRED", "not authenticated")
		return
	}

	// Verify membership
	var callerRole string
	err := h.pool.QueryRow(r.Context(), `
		SELECT om.role FROM org_memberships om WHERE om.user_id = $1 AND om.org_id = $2
	`, user.ID, orgID).Scan(&callerRole)
	if err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", "not a member of this org")
		return
	}

	var org models.Org
	err = h.pool.QueryRow(r.Context(), `SELECT id, name, slug, tier FROM orgs WHERE id = $1`, orgID).Scan(&org.ID, &org.Name, &org.Slug, &org.Tier)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "org not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	// Fetch members
	rows, err := h.pool.Query(r.Context(), `
		SELECT u.id, u.email, u.name, om.role, om.created_at
		FROM users u
		JOIN org_memberships om ON om.user_id = u.id
		WHERE om.org_id = $1
		ORDER BY om.created_at
	`, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	defer rows.Close()

	type member struct {
		ID       string `json:"id"`
		Email    string `json:"email"`
		Name     string `json:"name"`
		Role     string `json:"role"`
		JoinedAt string `json:"joined_at"`
	}
	var members []member
	for rows.Next() {
		var user models.User
		var m member
		var joinedAt time.Time
		if err := rows.Scan(&user.ID, &user.Email, &user.Name, &m.Role, &joinedAt); err != nil {
			continue
		}
		m.ID = user.ID
		m.Email = user.Email
		m.Name = user.Name
		m.JoinedAt = joinedAt.Format(time.RFC3339)
		members = append(members, m)
	}
	if members == nil {
		members = []member{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":      org.ID,
		"name":    org.Name,
		"slug":    org.Slug,
		"tier":    org.Tier,
		"role":    callerRole,
		"members": members,
	})
}

// PATCH /v1/orgs/:id
func (h *OrgHandlers) UpdateOrg(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	if _, err := requireOrgRole(r, h.pool, orgID, "owner", "admin"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	var body struct {
		Name *string `json:"name"`
		Slug *string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "invalid request body")
		return
	}
	if body.Name == nil && body.Slug == nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "name or slug is required")
		return
	}
	if body.Name != nil && *body.Name == "" {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "name cannot be empty")
		return
	}
	if body.Slug != nil && *body.Slug == "" {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "slug cannot be empty")
		return
	}

	var org models.Org
	err := h.pool.QueryRow(r.Context(), `
		UPDATE orgs
		SET
			name = COALESCE($1, name),
			slug = COALESCE($2, slug),
			updated_at = NOW()
		WHERE id = $3
		RETURNING id, name, slug
	`, body.Name, body.Slug, orgID).Scan(&org.ID, &org.Name, &org.Slug)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "SLUG_TAKEN", "an org with that slug already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":   org.ID,
		"name": org.Name,
		"slug": org.Slug,
	})
}

// POST /v1/orgs/:id/members — invite user by email
func (h *OrgHandlers) InviteMember(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	if _, err := requireOrgRole(r, h.pool, orgID, "owner", "admin"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	var body struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "email is required")
		return
	}
	if body.Role == "" {
		body.Role = "member"
	}
	if !isValidRole(body.Role) {
		writeError(w, http.StatusBadRequest, "INVALID_ROLE", "role must be owner, admin, member, or viewer")
		return
	}

	// Find user by email
	var user models.User
	err := h.pool.QueryRow(r.Context(), `SELECT id, email FROM users WHERE email = $1`, body.Email).Scan(&user.ID, &user.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "USER_NOT_FOUND", "no user with that email")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	_, err = h.pool.Exec(r.Context(), `
		INSERT INTO org_memberships (user_id, org_id, role) VALUES ($1, $2, $3)
		ON CONFLICT (user_id, org_id) DO UPDATE SET role = EXCLUDED.role
	`, user.ID, orgID, body.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"user_id": user.ID,
		"org_id":  orgID,
		"email":   user.Email,
		"role":    body.Role,
	})
}

// DELETE /v1/orgs/:id/members/:userId
func (h *OrgHandlers) RemoveMember(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userId")

	if _, err := requireOrgRole(r, h.pool, orgID, "owner", "admin"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	_, err := h.pool.Exec(r.Context(), `
		DELETE FROM org_memberships WHERE user_id = $1 AND org_id = $2
	`, targetUserID, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PATCH /v1/orgs/:id/members/:userId
func (h *OrgHandlers) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userId")

	if _, err := requireOrgRole(r, h.pool, orgID, "owner"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	var body struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Role == "" {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "role is required")
		return
	}
	if !isValidRole(body.Role) {
		writeError(w, http.StatusBadRequest, "INVALID_ROLE", "role must be owner, admin, member, or viewer")
		return
	}

	_, err := h.pool.Exec(r.Context(), `
		UPDATE org_memberships SET role = $1 WHERE user_id = $2 AND org_id = $3
	`, body.Role, targetUserID, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id": targetUserID,
		"org_id":  orgID,
		"role":    body.Role,
	})
}

func isValidRole(role string) bool {
	switch role {
	case "owner", "admin", "member", "viewer":
		return true
	}
	return false
}
