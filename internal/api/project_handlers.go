package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/XferOps/winnow/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProjectHandlers struct {
	pool *pgxpool.Pool
}

func NewProjectHandlers(pool *pgxpool.Pool) *ProjectHandlers {
	return &ProjectHandlers{pool: pool}
}

// POST /v1/orgs/:id/projects
func (h *ProjectHandlers) CreateProject(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	if _, err := requireOrgRole(r, h.pool, orgID, "owner", "admin"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
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

	var project models.Project
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO projects (org_id, name, slug) VALUES ($1, $2, $3) RETURNING id, org_id, name, slug
	`, orgID, body.Name, body.Slug).Scan(&project.ID, &project.OrgID, &project.Name, &project.Slug)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "SLUG_TAKEN", "a project with that slug already exists in this org")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":     project.ID,
		"org_id": project.OrgID,
		"name":   project.Name,
		"slug":   project.Slug,
	})
}

// GET /v1/orgs/:id/projects
func (h *ProjectHandlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	if _, err := requireOrgRole(r, h.pool, orgID, "owner", "admin", "member", "viewer"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT id, org_id, name, slug, created_at FROM projects WHERE org_id = $1 ORDER BY created_at
	`, orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	defer rows.Close()

	type projectItem struct {
		ID        string `json:"id"`
		OrgID     string `json:"org_id"`
		Name      string `json:"name"`
		Slug      string `json:"slug"`
		CreatedAt string `json:"created_at"`
	}
	var projects []projectItem
	for rows.Next() {
		var project models.Project
		if err := rows.Scan(&project.ID, &project.OrgID, &project.Name, &project.Slug, &project.CreatedAt); err != nil {
			continue
		}
		projects = append(projects, projectItem{
			ID:        project.ID,
			OrgID:     project.OrgID,
			Name:      project.Name,
			Slug:      project.Slug,
			CreatedAt: project.CreatedAt.Format(time.RFC3339),
		})
	}
	if projects == nil {
		projects = []projectItem{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"projects": projects})
}

// PATCH /v1/projects/:id
func (h *ProjectHandlers) UpdateProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	// Look up orgID for this project
	var project models.Project
	err := h.pool.QueryRow(r.Context(), `SELECT id, org_id FROM projects WHERE id = $1`, projectID).Scan(&project.ID, &project.OrgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	if _, err := requireOrgRole(r, h.pool, project.OrgID, "owner", "admin"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "name is required")
		return
	}

	_, err = h.pool.Exec(r.Context(), `UPDATE projects SET name = $1, updated_at = NOW() WHERE id = $2`, body.Name, project.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"id": project.ID, "org_id": project.OrgID, "name": body.Name})
}
