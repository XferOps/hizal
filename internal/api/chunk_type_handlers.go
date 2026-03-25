package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/XferOps/hizal/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ChunkTypeHandlers struct {
	pool *pgxpool.Pool
}

func NewChunkTypeHandlers(pool *pgxpool.Pool) *ChunkTypeHandlers {
	return &ChunkTypeHandlers{pool: pool}
}

func (h *ChunkTypeHandlers) ListChunkTypes(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	if _, err := requireOrgRole(r, h.pool, orgID, "owner", "admin", "member", "viewer"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT
			ct.id, ct.org_id, ct.name, ct.slug, ct.description,
			ct.default_scope, ct.default_inject_audience,
			ct.consolidation_behavior, ct.hidden,
			ct.created_at, ct.updated_at,
			g.id AS overrides_global_id
		FROM chunk_types ct
		LEFT JOIN chunk_types g
			ON g.slug = ct.slug AND g.org_id IS NULL AND ct.org_id = $1
		WHERE (
			-- org shadow rows (not hidden)
			(ct.org_id = $1 AND ct.hidden = false)
			OR
			-- globals with no org shadow
			(ct.org_id IS NULL AND NOT EXISTS (
				SELECT 1 FROM chunk_types shadow
				WHERE shadow.org_id = $1 AND shadow.slug = ct.slug
			))
		)
		ORDER BY ct.name
	`, orgID)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}
	defer rows.Close()

	var types []models.ChunkType
	for rows.Next() {
		var t models.ChunkType
		var overridesGlobalID *string
		if err := rows.Scan(
			&t.ID, &t.OrgID, &t.Name, &t.Slug, &t.Description,
			&t.DefaultScope, &t.DefaultInjectAudience, &t.ConsolidationBehavior,
			&t.Hidden, &t.CreatedAt, &t.UpdatedAt, &overridesGlobalID,
		); err != nil {
			continue
		}
		if overridesGlobalID != nil {
			t.OverridesGlobal = overridesGlobalID
		}
		types = append(types, t)
	}
	if types == nil {
		types = []models.ChunkType{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"chunk_types": types})
}

func (h *ChunkTypeHandlers) CreateChunkType(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	if _, err := requireOrgRole(r, h.pool, orgID, "owner", "admin"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	var body struct {
		Name                  string           `json:"name"`
		Slug                  string           `json:"slug"`
		Description           string           `json:"description"`
		DefaultScope          string           `json:"default_scope"`
		DefaultInjectAudience *json.RawMessage `json:"default_inject_audience"`
		ConsolidationBehavior string           `json:"consolidation_behavior"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeJSONDecodeError(w, err, "name and slug are required")
		return
	}
	if body.Name == "" || body.Slug == "" {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "name and slug are required")
		return
	}

	if body.DefaultScope == "" {
		body.DefaultScope = "PROJECT"
	}
	if body.ConsolidationBehavior == "" {
		body.ConsolidationBehavior = "SURFACE"
	}

	var t models.ChunkType
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO chunk_types (org_id, name, slug, description, default_scope, default_inject_audience, consolidation_behavior)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, org_id, name, slug, description, default_scope, default_inject_audience, consolidation_behavior, created_at, updated_at
	`, orgID, body.Name, body.Slug, nullableStr(body.Description), body.DefaultScope, nullJSONPtr(body.DefaultInjectAudience), body.ConsolidationBehavior).Scan(
		&t.ID, &t.OrgID, &t.Name, &t.Slug, &t.Description,
		&t.DefaultScope, &t.DefaultInjectAudience, &t.ConsolidationBehavior,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "SLUG_TAKEN", "a chunk type with that slug already exists in this org")
			return
		}
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	writeJSON(w, http.StatusCreated, t)
}

func (h *ChunkTypeHandlers) GetChunkType(w http.ResponseWriter, r *http.Request) {
	typeID := chi.URLParam(r, "id")

	var t models.ChunkType
	err := h.pool.QueryRow(r.Context(), `
		SELECT id, org_id, name, slug, description, default_scope, default_inject_audience, consolidation_behavior, created_at, updated_at
		FROM chunk_types WHERE id = $1
	`, typeID).Scan(
		&t.ID, &t.OrgID, &t.Name, &t.Slug, &t.Description,
		&t.DefaultScope, &t.DefaultInjectAudience, &t.ConsolidationBehavior,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "chunk type not found")
		return
	}

	if t.OrgID != nil {
		if _, err := requireOrgRole(r, h.pool, *t.OrgID, "owner", "admin", "member", "viewer"); err != nil {
			writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, t)
}

func (h *ChunkTypeHandlers) UpdateChunkType(w http.ResponseWriter, r *http.Request) {
	typeID := chi.URLParam(r, "id")

	var orgID *string
	err := h.pool.QueryRow(r.Context(), `SELECT org_id FROM chunk_types WHERE id = $1`, typeID).Scan(&orgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "chunk type not found")
		return
	}

	if orgID == nil {
		writeError(w, http.StatusForbidden, "GLOBAL_TYPE", "global presets cannot be modified")
		return
	}

	if _, err := requireOrgRole(r, h.pool, *orgID, "owner", "admin"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	var body struct {
		Name                  *string          `json:"name"`
		Description           *string          `json:"description"`
		DefaultScope          *string          `json:"default_scope"`
		DefaultInjectAudience *json.RawMessage `json:"default_inject_audience"`
		ConsolidationBehavior *string          `json:"consolidation_behavior"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeJSONDecodeError(w, err, "")
		return
	}

	setClauses := []string{}
	args := []any{}
	idx := 2

	if body.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", idx))
		args = append(args, *body.Name)
		idx++
	}
	if body.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", idx))
		args = append(args, *body.Description)
		idx++
	}
	if body.DefaultScope != nil {
		setClauses = append(setClauses, fmt.Sprintf("default_scope = $%d", idx))
		args = append(args, *body.DefaultScope)
		idx++
	}
	if body.DefaultInjectAudience != nil {
		setClauses = append(setClauses, fmt.Sprintf("default_inject_audience = $%d", idx))
		args = append(args, nullJSONPtr(body.DefaultInjectAudience))
		idx++
	}
	if body.ConsolidationBehavior != nil {
		setClauses = append(setClauses, fmt.Sprintf("consolidation_behavior = $%d", idx))
		args = append(args, *body.ConsolidationBehavior)
		idx++
	}

	if len(setClauses) > 0 {
		setClauses = append(setClauses, "updated_at = NOW()")
		args = append(args, typeID)
		query := fmt.Sprintf("UPDATE chunk_types SET %s WHERE id = $%d", joinStrings(setClauses, ", "), idx)
		_, err = h.pool.Exec(r.Context(), query, args...)
		if err != nil {
			writeInternalError(r, w, "DB_ERROR", err)
			return
		}
	}

	var t models.ChunkType
	err = h.pool.QueryRow(r.Context(), `
		SELECT id, org_id, name, slug, description, default_scope, default_inject_audience, consolidation_behavior, created_at, updated_at
		FROM chunk_types WHERE id = $1
	`, typeID).Scan(
		&t.ID, &t.OrgID, &t.Name, &t.Slug, &t.Description,
		&t.DefaultScope, &t.DefaultInjectAudience, &t.ConsolidationBehavior,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	writeJSON(w, http.StatusOK, t)
}

func (h *ChunkTypeHandlers) DeleteChunkType(w http.ResponseWriter, r *http.Request) {
	typeID := chi.URLParam(r, "id")

	var orgID *string
	err := h.pool.QueryRow(r.Context(), `SELECT org_id FROM chunk_types WHERE id = $1`, typeID).Scan(&orgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "chunk type not found")
		return
	}

	if orgID == nil {
		writeError(w, http.StatusForbidden, "GLOBAL_TYPE", "global presets cannot be deleted")
		return
	}

	if _, err := requireOrgRole(r, h.pool, *orgID, "owner", "admin"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	_, err = h.pool.Exec(r.Context(), `DELETE FROM chunk_types WHERE id = $1`, typeID)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ChunkTypeHandlers) ForkOverride(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	slug := chi.URLParam(r, "slug")

	if _, err := requireOrgRole(r, h.pool, orgID, "owner", "admin"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	var globalID string
	var globalName, globalSlug, globalDesc, globalScope, globalConsolidation string
	var globalInjectAudience *json.RawMessage
	err := h.pool.QueryRow(r.Context(), `
		SELECT id, name, slug, description, default_scope, default_inject_audience, consolidation_behavior
		FROM chunk_types WHERE org_id IS NULL AND slug = $1
	`, slug).Scan(
		&globalID, &globalName, &globalSlug, &globalDesc,
		&globalScope, &globalInjectAudience, &globalConsolidation,
	)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "global chunk type not found")
		return
	}

	var existingID string
	err = h.pool.QueryRow(r.Context(), `SELECT id FROM chunk_types WHERE org_id = $1 AND slug = $2`, orgID, slug).Scan(&existingID)
	if err == nil {
		writeError(w, http.StatusConflict, "ALREADY_OVERRIDDEN", "org already has an override for this chunk type")
		return
	}

	var t models.ChunkType
	err = h.pool.QueryRow(r.Context(), `
		INSERT INTO chunk_types (org_id, name, slug, description, default_scope, default_inject_audience, consolidation_behavior, hidden)
		VALUES ($1, $2, $3, $4, $5, $6, $7, false)
		RETURNING id, org_id, name, slug, description, default_scope, default_inject_audience, consolidation_behavior, hidden, created_at, updated_at
	`, orgID, globalName, globalSlug, nullableStr(globalDesc), globalScope, nullJSONPtr(globalInjectAudience), globalConsolidation).Scan(
		&t.ID, &t.OrgID, &t.Name, &t.Slug, &t.Description,
		&t.DefaultScope, &t.DefaultInjectAudience, &t.ConsolidationBehavior,
		&t.Hidden, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	t.OverridesGlobal = &globalID
	writeJSON(w, http.StatusCreated, t)
}

func (h *ChunkTypeHandlers) ResetOrHide(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	slug := chi.URLParam(r, "slug")
	action := r.URL.Query().Get("action")

	if _, err := requireOrgRole(r, h.pool, orgID, "owner", "admin"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	var globalID string
	err := h.pool.QueryRow(r.Context(), `SELECT id FROM chunk_types WHERE org_id IS NULL AND slug = $1`, slug).Scan(&globalID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "global chunk type not found")
		return
	}

	if action == "hide" {
		var existingID string
		var existingHidden bool
		err = h.pool.QueryRow(r.Context(), `SELECT id, hidden FROM chunk_types WHERE org_id = $1 AND slug = $2`, orgID, slug).Scan(&existingID, &existingHidden)
		if err == nil {
			if existingHidden {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			_, err = h.pool.Exec(r.Context(), `UPDATE chunk_types SET hidden = true WHERE id = $1`, existingID)
			if err != nil {
				writeInternalError(r, w, "DB_ERROR", err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		var globalName, globalSlug, globalDesc, globalScope, globalConsolidation string
		var globalInjectAudience *json.RawMessage
		err = h.pool.QueryRow(r.Context(), `
			SELECT name, slug, description, default_scope, default_inject_audience, consolidation_behavior
			FROM chunk_types WHERE org_id IS NULL AND slug = $1
		`, slug).Scan(
			&globalName, &globalSlug, &globalDesc,
			&globalScope, &globalInjectAudience, &globalConsolidation,
		)
		if err != nil {
			writeInternalError(r, w, "DB_ERROR", err)
			return
		}

		_, err = h.pool.Exec(r.Context(), `
			INSERT INTO chunk_types (org_id, name, slug, description, default_scope, default_inject_audience, consolidation_behavior, hidden)
			VALUES ($1, $2, $3, $4, $5, $6, $7, true)
		`, orgID, globalName, globalSlug, nullableStr(globalDesc), globalScope, nullJSONPtr(globalInjectAudience), globalConsolidation)
		if err != nil {
			writeInternalError(r, w, "DB_ERROR", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		return
	}

	_, err = h.pool.Exec(r.Context(), `DELETE FROM chunk_types WHERE org_id = $1 AND slug = $2`, orgID, slug)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func nullJSONPtr(raw *json.RawMessage) interface{} {
	if raw == nil || len(*raw) == 0 || string(*raw) == "null" {
		return nil
	}
	return string(*raw)
}

func joinStrings(parts []string, sep string) string {
	return strings.Join(parts, sep)
}
