package api

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AdminHandlers struct {
	pool *pgxpool.Pool
}

func NewAdminHandlers(pool *pgxpool.Pool) *AdminHandlers {
	return &AdminHandlers{pool: pool}
}

type AdminOrg struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	OwnerEmail   *string   `json:"owner_email"`
	CreatedAt    time.Time `json:"created_at"`
	Tier         string    `json:"tier"`
	ProjectCount int       `json:"project_count"`
}

type AdminUser struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	CreatedAt   time.Time  `json:"created_at"`
	OrgCount    int        `json:"org_count"`
	LastLoginAt *time.Time `json:"last_login_at"`
}

type AdminProject struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	OrgID      string    `json:"org_id"`
	OrgName    string    `json:"org_name"`
	ChunkCount int       `json:"chunk_count"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type AdminKey struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	UserEmail  *string    `json:"user_email"`
	OrgID      *string    `json:"org_id"`
	OrgName    *string    `json:"org_name"`
}

func nullableStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}

	return &value.String
}

func nullableTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}

	return &value.Time
}

func (h *AdminHandlers) ListOrgs(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT 
			o.id, o.name, o.slug,
			u.email AS owner_email,
			o.created_at,
			o.tier,
			COUNT(DISTINCT p.id) AS project_count
		FROM orgs o
		LEFT JOIN org_memberships om ON om.org_id = o.id AND om.role = 'owner'
		LEFT JOIN users u ON u.id = om.user_id
		LEFT JOIN projects p ON p.org_id = o.id
		GROUP BY o.id, u.email
		ORDER BY o.created_at DESC
	`)
	if err != nil {
		writeInternalError(r, w, "QUERY_FAILED", err)
		return
	}
	defer rows.Close()

	var orgs []AdminOrg
	for rows.Next() {
		var org AdminOrg
		var ownerEmail sql.NullString
		if err := rows.Scan(
			&org.ID, &org.Name, &org.Slug,
			&ownerEmail, &org.CreatedAt,
			&org.Tier, &org.ProjectCount,
		); err != nil {
			writeInternalError(r, w, "SCAN_FAILED", err)
			return
		}
		org.OwnerEmail = nullableStringPtr(ownerEmail)
		orgs = append(orgs, org)
	}
	writeJSON(w, http.StatusOK, orgs)
}

func (h *AdminHandlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT
			u.id, u.email, u.name, u.created_at,
			COUNT(DISTINCT om.org_id) AS org_count,
			MAX(al.created_at) AS last_login_at
		FROM users u
		LEFT JOIN org_memberships om ON om.user_id = u.id
		LEFT JOIN audit_log al ON al.actor_id = u.id AND al.action = 'LOGIN_SUCCESS'
		GROUP BY u.id
		ORDER BY u.created_at DESC
	`)
	if err != nil {
		writeInternalError(r, w, "QUERY_FAILED", err)
		return
	}
	defer rows.Close()

	var users []AdminUser
	for rows.Next() {
		var user AdminUser
		var lastLoginAt sql.NullTime
		if err := rows.Scan(
			&user.ID, &user.Email, &user.Name, &user.CreatedAt,
			&user.OrgCount, &lastLoginAt,
		); err != nil {
			writeInternalError(r, w, "SCAN_FAILED", err)
			return
		}
		user.LastLoginAt = nullableTimePtr(lastLoginAt)
		users = append(users, user)
	}
	writeJSON(w, http.StatusOK, users)
}

func (h *AdminHandlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT
			p.id, p.name, p.slug, p.org_id,
			o.name AS org_name,
			COUNT(DISTINCT c.id) AS chunk_count,
			p.updated_at
		FROM projects p
		LEFT JOIN orgs o ON o.id = p.org_id
		LEFT JOIN context_chunks c ON c.project_id = p.id
		GROUP BY p.id, o.name
		ORDER BY p.updated_at DESC
	`)
	if err != nil {
		writeInternalError(r, w, "QUERY_FAILED", err)
		return
	}
	defer rows.Close()

	var projects []AdminProject
	for rows.Next() {
		var project AdminProject
		if err := rows.Scan(
			&project.ID, &project.Name, &project.Slug, &project.OrgID,
			&project.OrgName, &project.ChunkCount, &project.UpdatedAt,
		); err != nil {
			writeInternalError(r, w, "SCAN_FAILED", err)
			return
		}
		projects = append(projects, project)
	}
	writeJSON(w, http.StatusOK, projects)
}

func (h *AdminHandlers) ListKeys(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT
			ak.id, ak.name, ak.created_at, ak.last_used_at,
			u.email AS user_email,
			o.id AS org_id, o.name AS org_name
		FROM api_keys ak
		LEFT JOIN users u ON u.id = ak.user_id
		LEFT JOIN orgs o ON o.id = ak.org_id
		ORDER BY ak.created_at DESC
	`)
	if err != nil {
		writeInternalError(r, w, "QUERY_FAILED", err)
		return
	}
	defer rows.Close()

	var keys []AdminKey
	for rows.Next() {
		var key AdminKey
		var lastUsedAt sql.NullTime
		var userEmail sql.NullString
		var orgID sql.NullString
		var orgName sql.NullString
		if err := rows.Scan(
			&key.ID, &key.Name, &key.CreatedAt, &lastUsedAt,
			&userEmail, &orgID, &orgName,
		); err != nil {
			writeInternalError(r, w, "SCAN_FAILED", err)
			return
		}
		key.LastUsedAt = nullableTimePtr(lastUsedAt)
		key.UserEmail = nullableStringPtr(userEmail)
		key.OrgID = nullableStringPtr(orgID)
		key.OrgName = nullableStringPtr(orgName)
		keys = append(keys, key)
	}
	writeJSON(w, http.StatusOK, keys)
}
