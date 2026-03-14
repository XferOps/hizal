package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/XferOps/winnow/internal/mcp"
	"github.com/XferOps/winnow/internal/seed"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedHandlers handles the auto-seed endpoint.
type SeedHandlers struct {
	pool  *pgxpool.Pool
	tools *mcp.Tools
}

func NewSeedHandlers(pool *pgxpool.Pool, tools *mcp.Tools) *SeedHandlers {
	return &SeedHandlers{pool: pool, tools: tools}
}

// POST /v1/projects/{id}/seed
//
// Body: { "repo_url": "https://github.com/owner/repo", "github_token": "optional" }
// Response: text/event-stream (SSE via streaming fetch)
//
// SSE event types:
//
//	event: progress  data: {"step":"...","message":"...","current":N,"total":N,"category":"..."}
//	event: complete  data: {"chunks_written":N,"categories":["..."]}
//	event: error     data: {"code":"...","message":"..."}
func (h *SeedHandlers) SeedProject(w http.ResponseWriter, r *http.Request) {
	projectID := chi.URLParam(r, "id")

	if h.pool == nil {
		writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database not connected")
		return
	}

	// Resolve org from project and require owner/admin
	var orgID string
	if err := h.pool.QueryRow(r.Context(), `SELECT org_id FROM projects WHERE id = $1`, projectID).Scan(&orgID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	if _, err := requireOrgRole(r, h.pool, orgID, "owner", "admin"); err != nil {
		writeError(w, http.StatusForbidden, "FORBIDDEN", err.Error())
		return
	}

	var body struct {
		RepoURL     string `json:"repo_url"`
		GitHubToken string `json:"github_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "repo_url is required")
		return
	}

	// Set SSE headers before writing anything
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "STREAMING_UNSUPPORTED", "streaming not supported by this server")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	events := seed.Run(r.Context(), h.tools, seed.Request{
		RepoURL:     body.RepoURL,
		GitHubToken: body.GitHubToken,
		ProjectID:   projectID,
	})

	for event := range events {
		dataBytes, err := json.Marshal(event.Data)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, dataBytes)
		flusher.Flush()
	}
}
