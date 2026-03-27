package api

import (
	"encoding/json"
	"net/http"

	"github.com/XferOps/hizal/internal/models"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AgentTypeHandlers struct {
	pool *pgxpool.Pool
}

func NewAgentTypeHandlers(pool *pgxpool.Pool) *AgentTypeHandlers {
	return &AgentTypeHandlers{pool: pool}
}

func (h *AgentTypeHandlers) ListAgentTypes(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	if _, err := requireOrgRole(r, h.pool, orgID, "owner", "admin", "member", "viewer"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT
			at.id, at.org_id, at.name, at.slug, at.base_type, at.description,
			at.inject_filters, at.search_filters, at.hidden, at.created_at, at.updated_at,
			g.id AS overrides_global_id
		FROM agent_types at
		LEFT JOIN agent_types g
			ON g.slug = at.slug AND g.org_id IS NULL AND at.org_id = $1
		WHERE (
			(at.org_id = $1 AND at.hidden = false)
			OR
			(at.org_id IS NULL AND NOT EXISTS (
				SELECT 1 FROM agent_types shadow
				WHERE shadow.org_id = $1 AND shadow.slug = at.slug
			))
		)
		ORDER BY at.org_id NULLS FIRST, at.name
	`, orgID)
	if err != nil {
		writeInternalError(r, w, "QUERY_FAILED", err)
		return
	}
	defer rows.Close()

	var types []models.AgentType
	for rows.Next() {
		var t models.AgentType
		var injectFiltersJSON, searchFiltersJSON []byte
		var overridesGlobalID *string
		if err := rows.Scan(
			&t.ID, &t.OrgID, &t.Name, &t.Slug, &t.BaseType, &t.Description,
			&injectFiltersJSON, &searchFiltersJSON, &t.Hidden, &t.CreatedAt, &t.UpdatedAt,
			&overridesGlobalID,
		); err != nil {
			continue
		}
		json.Unmarshal(injectFiltersJSON, &t.InjectFilters)
		json.Unmarshal(searchFiltersJSON, &t.SearchFilters)
		t.IsGlobal = t.OrgID == nil
		if overridesGlobalID != nil {
			t.OverridesGlobal = overridesGlobalID
		}
		types = append(types, t)
	}
	if types == nil {
		types = []models.AgentType{}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"agent_types": types})
}

// POST /v1/orgs/:id/agent-types/:slug/fork
func (h *AgentTypeHandlers) ForkAgentType(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	slug := chi.URLParam(r, "slug")

	if _, err := requireOrgRole(r, h.pool, orgID, "owner", "admin"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	var globalID string
	var globalName, globalSlug string
	var globalBaseType, globalDesc *string
	var globalInjectFilters, globalSearchFilters []byte
	err := h.pool.QueryRow(r.Context(), `
		SELECT id, name, slug, base_type, description, inject_filters, search_filters
		FROM agent_types WHERE org_id IS NULL AND slug = $1
	`, slug).Scan(
		&globalID, &globalName, &globalSlug, &globalBaseType, &globalDesc,
		&globalInjectFilters, &globalSearchFilters,
	)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "global agent type not found")
		return
	}

	var existingID string
	err = h.pool.QueryRow(r.Context(), `SELECT id FROM agent_types WHERE org_id = $1 AND slug = $2`, orgID, slug).Scan(&existingID)
	if err == nil {
		writeError(w, http.StatusConflict, "ALREADY_OVERRIDDEN", "org already has an override for this agent type")
		return
	}

	var t models.AgentType
	err = h.pool.QueryRow(r.Context(), `
		INSERT INTO agent_types (org_id, name, slug, base_type, description, inject_filters, search_filters, hidden)
		VALUES ($1, $2, $3, $4, $5, $6, $7, false)
		RETURNING id, org_id, name, slug, base_type, description, inject_filters, search_filters, hidden, created_at, updated_at
	`, orgID, globalName, globalSlug, globalBaseType, globalDesc, globalInjectFilters, globalSearchFilters).Scan(
		&t.ID, &t.OrgID, &t.Name, &t.Slug, &t.BaseType, &t.Description,
		&globalInjectFilters, &globalSearchFilters, &t.Hidden, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	json.Unmarshal(globalInjectFilters, &t.InjectFilters)
	json.Unmarshal(globalSearchFilters, &t.SearchFilters)
	t.IsGlobal = false
	t.OverridesGlobal = &globalID

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"agent_type":        t,
		"overrides_global":  globalID,
	})
}

// POST /v1/orgs/:id/agent-types/:slug/reset?action=hide|reset
func (h *AgentTypeHandlers) ResetOrHideAgentType(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	slug := chi.URLParam(r, "slug")
	action := r.URL.Query().Get("action")

	if _, err := requireOrgRole(r, h.pool, orgID, "owner", "admin"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	var globalID string
	err := h.pool.QueryRow(r.Context(), `SELECT id FROM agent_types WHERE org_id IS NULL AND slug = $1`, slug).Scan(&globalID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "global agent type not found")
		return
	}

	if action == "hide" {
		var existingID string
		var existingHidden bool
		err = h.pool.QueryRow(r.Context(), `SELECT id, hidden FROM agent_types WHERE org_id = $1 AND slug = $2`, orgID, slug).Scan(&existingID, &existingHidden)
		if err == nil {
			if existingHidden {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			_, err = h.pool.Exec(r.Context(), `UPDATE agent_types SET hidden = true WHERE id = $1`, existingID)
			if err != nil {
				writeInternalError(r, w, "DB_ERROR", err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		var globalName, globalSlug string
		var globalBaseType, globalDesc *string
		var globalInjectFilters, globalSearchFilters []byte
		err = h.pool.QueryRow(r.Context(), `
			SELECT name, slug, base_type, description, inject_filters, search_filters
			FROM agent_types WHERE org_id IS NULL AND slug = $1
		`, slug).Scan(
			&globalName, &globalSlug, &globalBaseType, &globalDesc,
			&globalInjectFilters, &globalSearchFilters,
		)
		if err != nil {
			writeInternalError(r, w, "DB_ERROR", err)
			return
		}

		_, err = h.pool.Exec(r.Context(), `
			INSERT INTO agent_types (org_id, name, slug, base_type, description, inject_filters, search_filters, hidden)
			VALUES ($1, $2, $3, $4, $5, $6, $7, true)
		`, orgID, globalName, globalSlug, globalBaseType, globalDesc, globalInjectFilters, globalSearchFilters)
		if err != nil {
			writeInternalError(r, w, "DB_ERROR", err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		return
	}

	_, err = h.pool.Exec(r.Context(), `DELETE FROM agent_types WHERE org_id = $1 AND slug = $2`, orgID, slug)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AgentTypeHandlers) CreateAgentType(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	if _, err := requireOrgRole(r, h.pool, orgID, "owner", "admin"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	var body struct {
		Name          string                       `json:"name"`
		Slug          string                       `json:"slug"`
		BaseType      string                       `json:"base_type"`
		Description   string                       `json:"description"`
		InjectFilters models.AgentTypeFilterConfig `json:"inject_filters"`
		SearchFilters models.AgentTypeFilterConfig `json:"search_filters"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeJSONDecodeError(w, err, "name and slug are required")
		return
	}
	if body.Name == "" || body.Slug == "" {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "name and slug are required")
		return
	}

	injectFiltersJSON, _ := json.Marshal(body.InjectFilters)
	searchFiltersJSON, _ := json.Marshal(body.SearchFilters)

	var t models.AgentType
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO agent_types (org_id, name, slug, base_type, description, inject_filters, search_filters)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, org_id, name, slug, base_type, description, inject_filters, search_filters, created_at, updated_at
	`, orgID, body.Name, body.Slug, nullableStr(body.BaseType), nullableStr(body.Description),
		injectFiltersJSON, searchFiltersJSON).Scan(
		&t.ID, &t.OrgID, &t.Name, &t.Slug, &t.BaseType, &t.Description,
		&injectFiltersJSON, &searchFiltersJSON, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "SLUG_TAKEN", "an agent type with that slug already exists in this org")
			return
		}
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}
	json.Unmarshal(injectFiltersJSON, &t.InjectFilters)
	json.Unmarshal(searchFiltersJSON, &t.SearchFilters)

	writeJSON(w, http.StatusCreated, t)
}

func (h *AgentTypeHandlers) GetAgentType(w http.ResponseWriter, r *http.Request) {
	typeID := chi.URLParam(r, "id")

	var t models.AgentType
	var injectFiltersJSON, searchFiltersJSON []byte
	err := h.pool.QueryRow(r.Context(), `
		SELECT id, org_id, name, slug, base_type, description, inject_filters, search_filters, created_at, updated_at
		FROM agent_types WHERE id = $1
	`, typeID).Scan(
		&t.ID, &t.OrgID, &t.Name, &t.Slug, &t.BaseType, &t.Description,
		&injectFiltersJSON, &searchFiltersJSON, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "agent type not found")
		return
	}
	json.Unmarshal(injectFiltersJSON, &t.InjectFilters)
	json.Unmarshal(searchFiltersJSON, &t.SearchFilters)

	if t.OrgID != nil {
		if _, err := requireOrgRole(r, h.pool, *t.OrgID, "owner", "admin", "member", "viewer"); err != nil {
			writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, t)
}

func (h *AgentTypeHandlers) UpdateAgentType(w http.ResponseWriter, r *http.Request) {
	typeID := chi.URLParam(r, "id")

	var orgID *string
	err := h.pool.QueryRow(r.Context(), `SELECT org_id FROM agent_types WHERE id = $1`, typeID).Scan(&orgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "agent type not found")
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
		Name          *string                       `json:"name"`
		Description   *string                       `json:"description"`
		InjectFilters *models.AgentTypeFilterConfig `json:"inject_filters"`
		SearchFilters *models.AgentTypeFilterConfig `json:"search_filters"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeJSONDecodeError(w, err, "")
		return
	}

	var injectFiltersJSON, searchFiltersJSON []byte
	if body.InjectFilters != nil {
		injectFiltersJSON, _ = json.Marshal(body.InjectFilters)
	}
	if body.SearchFilters != nil {
		searchFiltersJSON, _ = json.Marshal(body.SearchFilters)
	}

	_, err = h.pool.Exec(r.Context(), `
		UPDATE agent_types SET
		  name           = COALESCE($2, name),
		  description    = COALESCE($3, description),
		  inject_filters = COALESCE($4, inject_filters),
		  search_filters = COALESCE($5, search_filters),
		  updated_at     = NOW()
		WHERE id = $1
	`, typeID, body.Name, body.Description,
		nullableBytes(injectFiltersJSON), nullableBytes(searchFiltersJSON))
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}

	var t models.AgentType
	err = h.pool.QueryRow(r.Context(), `
		SELECT id, org_id, name, slug, base_type, description, inject_filters, search_filters, created_at, updated_at
		FROM agent_types WHERE id = $1
	`, typeID).Scan(
		&t.ID, &t.OrgID, &t.Name, &t.Slug, &t.BaseType, &t.Description,
		&injectFiltersJSON, &searchFiltersJSON, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}
	json.Unmarshal(injectFiltersJSON, &t.InjectFilters)
	json.Unmarshal(searchFiltersJSON, &t.SearchFilters)

	writeJSON(w, http.StatusOK, t)
}

func (h *AgentTypeHandlers) DeleteAgentType(w http.ResponseWriter, r *http.Request) {
	typeID := chi.URLParam(r, "id")

	var orgID *string
	err := h.pool.QueryRow(r.Context(), `SELECT org_id FROM agent_types WHERE id = $1`, typeID).Scan(&orgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "agent type not found")
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

	_, err = h.pool.Exec(r.Context(), `DELETE FROM agent_types WHERE id = $1`, typeID)
	if err != nil {
		writeInternalError(r, w, "DB_ERROR", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func nullableBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b
}
