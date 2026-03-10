package api

import (
	"encoding/json"
	"net/http"

	"github.com/XferOps/winnow/internal/embeddings"
	"github.com/XferOps/winnow/internal/mcp"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

const version = "0.1.0"

func NewRouter(pool *pgxpool.Pool, embed *embeddings.Client) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(corsMiddleware)

	r.Get("/health", healthHandler)

	var h *Handlers
	var mcpServer *mcp.Server
	if pool != nil {
		mcpServer = mcp.NewServer(pool, embed)
		h = NewHandlers(mcpServer.Tools(), pool)
	}

	authH := NewAuthHandlers(pool)
	orgH := NewOrgHandlers(pool)
	projH := NewProjectHandlers(pool)
	keyH := NewKeyHandlers(pool)

	// ── Auth routes (no auth required for register/login) ──────────────────
	r.Route("/v1/auth", func(r chi.Router) {
		r.Post("/register", authH.Register)
		r.Post("/login", authH.Login)
		r.With(JWTAuth()).Get("/me", authH.Me)
	})

	// ── Bootstrap key creation (kept for backward compat, no auth required) ──
	r.Post("/v1/keys", func(w http.ResponseWriter, r *http.Request) {
		// If JWT present, route to new key handler; otherwise legacy bootstrap
		if _, err := ParseJWT(extractBearer(r)); err == nil {
			// JWT path: inject user into context then call new handler
			claims, _ := ParseJWT(extractBearer(r))
			user := JWTUser{ID: claims.UserID, Email: claims.Email}
			keyH.CreateKey(w, r.WithContext(withJWTUser(r.Context(), user)))
		} else {
			if h == nil {
				writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database not connected")
				return
			}
			h.CreateAPIKey(w, r)
		}
	})

	// ── JWT-protected routes ────────────────────────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(JWTAuth())

		// Orgs
		r.Post("/v1/orgs", orgH.CreateOrg)
		r.Get("/v1/orgs", orgH.ListOrgs)
		r.Get("/v1/orgs/{id}", orgH.GetOrg)
		r.Patch("/v1/orgs/{id}", orgH.UpdateOrg)
		r.Post("/v1/orgs/{id}/members", orgH.InviteMember)
		r.Delete("/v1/orgs/{id}/members/{userId}", orgH.RemoveMember)
		r.Patch("/v1/orgs/{id}/members/{userId}", orgH.UpdateMemberRole)

		// Projects
		r.Post("/v1/orgs/{id}/projects", projH.CreateProject)
		r.Get("/v1/orgs/{id}/projects", projH.ListProjects)
		r.Patch("/v1/projects/{id}", projH.UpdateProject)

		// API keys
		r.Get("/v1/keys", keyH.ListKeys)
		r.Delete("/v1/keys/{id}", keyH.DeleteKey)
	})

	// MCP JSON-RPC endpoint (requires API key auth)
	r.With(APIKeyAuth(pool)).Post("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if mcpServer == nil {
			writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database not connected")
			return
		}
		mcpServer.ServeHTTP(w, r)
	})

	// REST API (requires API key auth)
	r.Route("/v1/context", func(r chi.Router) {
		r.Use(APIKeyAuth(pool))

		r.Post("/", func(w http.ResponseWriter, r *http.Request) {
			if h == nil {
				writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database not connected")
				return
			}
			h.WriteContext(w, r)
		})
		r.Get("/search", func(w http.ResponseWriter, r *http.Request) {
			if h == nil {
				writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database not connected")
				return
			}
			h.SearchContext(w, r)
		})
		r.Get("/compact", func(w http.ResponseWriter, r *http.Request) {
			if h == nil {
				writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database not connected")
				return
			}
			h.CompactContext(w, r)
		})
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				if h == nil {
					writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database not connected")
					return
				}
				h.ReadContext(w, r)
			})
			r.Get("/versions", func(w http.ResponseWriter, r *http.Request) {
				if h == nil {
					writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database not connected")
					return
				}
				h.GetContextVersions(w, r)
			})
			r.Patch("/", func(w http.ResponseWriter, r *http.Request) {
				if h == nil {
					writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database not connected")
					return
				}
				h.UpdateContext(w, r)
			})
			r.Delete("/", func(w http.ResponseWriter, r *http.Request) {
				if h == nil {
					writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database not connected")
					return
				}
				h.DeleteContext(w, r)
			})
			r.Post("/review", func(w http.ResponseWriter, r *http.Request) {
				if h == nil {
					writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database not connected")
					return
				}
				h.ReviewContext(w, r)
			})
		})
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": version,
	})
}

func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && h[:7] == "Bearer " {
		return h[7:]
	}
	return ""
}
