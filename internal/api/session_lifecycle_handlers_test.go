package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCreateAndListSessionLifecycles(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("pool.Ping() error = %v", err)
	}

	orgID := uuid.NewString()
	userID := uuid.NewString()
	orgMembershipID := uuid.NewString()
	email := "lc-test-" + strings.ToLower(uuid.NewString()) + "@example.com"
	orgSlug := "lc-test-org-" + strings.ToLower(uuid.NewString())

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM session_lifecycles WHERE org_id = $1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM orgs WHERE id = $1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	if _, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, $2, $3)`, orgID, "LC Test Org", orgSlug); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, $3)`, userID, email, "LC Test User"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO org_memberships (id, user_id, org_id, role) VALUES ($1, $2, $3, 'admin')`, orgMembershipID, userID, orgID); err != nil {
		t.Fatalf("insert org membership: %v", err)
	}

	t.Run("CreateSessionLifecycle creates org-scoped lifecycle", func(t *testing.T) {
		body := `{"name": "Test Lifecycle", "slug": "test-lc", "description": "A test lifecycle", "config": {"ttl_hours": 4, "inject_scopes": ["AGENT", "PROJECT"]}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/orgs/"+orgID+"/session-lifecycles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", orgID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: userID, Email: email}))

		rr := httptest.NewRecorder()
		NewSessionHandlers(nil, pool).CreateSessionLifecycle(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusCreated, rr.Body.String())
		}

		var created struct {
			ID        string      `json:"id"`
			OrgID     string      `json:"org_id"`
			Name      string      `json:"name"`
			Slug      string      `json:"slug"`
			IsGlobal  bool        `json:"is_global"`
			IsDefault bool        `json:"is_default"`
			Config    interface{} `json:"config"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		if created.OrgID != orgID {
			t.Fatalf("org_id = %q, want %q", created.OrgID, orgID)
		}
		if created.Slug != "test-lc" {
			t.Fatalf("slug = %q, want %q", created.Slug, "test-lc")
		}
		if created.IsGlobal {
			t.Fatalf("is_global = true, want false")
		}
	})

	t.Run("CreateSessionLifecycle rejects duplicate slug", func(t *testing.T) {
		body := `{"name": "Another", "slug": "test-lc", "config": {"ttl_hours": 4, "inject_scopes": ["AGENT"]}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/orgs/"+orgID+"/session-lifecycles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", orgID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: userID, Email: email}))

		rr := httptest.NewRecorder()
		NewSessionHandlers(nil, pool).CreateSessionLifecycle(rr, req)

		if rr.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusConflict, rr.Body.String())
		}
	})

	t.Run("CreateSessionLifecycle rejects invalid slug", func(t *testing.T) {
		body := `{"name": "Bad Slug", "slug": "Bad_Slug!", "config": {"ttl_hours": 4}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/orgs/"+orgID+"/session-lifecycles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", orgID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: userID, Email: email}))

		rr := httptest.NewRecorder()
		NewSessionHandlers(nil, pool).CreateSessionLifecycle(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
		}
	})

	t.Run("CreateSessionLifecycle rejects ttl_hours <= 0", func(t *testing.T) {
		body := `{"name": "Bad TTL", "slug": "bad-ttl", "config": {"ttl_hours": 0}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/orgs/"+orgID+"/session-lifecycles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", orgID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: userID, Email: email}))

		rr := httptest.NewRecorder()
		NewSessionHandlers(nil, pool).CreateSessionLifecycle(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
		}
	})

	t.Run("CreateSessionLifecycle rejects invalid inject_scopes", func(t *testing.T) {
		body := `{"name": "Bad Scope", "slug": "bad-scope", "config": {"ttl_hours": 4, "inject_scopes": ["INVALID"]}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/orgs/"+orgID+"/session-lifecycles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", orgID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: userID, Email: email}))

		rr := httptest.NewRecorder()
		NewSessionHandlers(nil, pool).CreateSessionLifecycle(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
		}
	})

	t.Run("CreateSessionLifecycle requires admin role", func(t *testing.T) {
		viewerUserID := uuid.NewString()
		viewerEmail := "lc-viewer-" + strings.ToLower(uuid.NewString()) + "@example.com"
		viewerMembershipID := uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, $3)`, viewerUserID, viewerEmail, "LC Viewer"); err != nil {
			t.Fatalf("insert viewer user: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO org_memberships (id, user_id, org_id, role) VALUES ($1, $2, $3, 'member')`, viewerMembershipID, viewerUserID, orgID); err != nil {
			t.Fatalf("insert viewer membership: %v", err)
		}
		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM org_memberships WHERE user_id = $1`, viewerUserID)
			_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, viewerUserID)
		})

		body := `{"name": "Viewer Test", "slug": "viewer-test", "config": {"ttl_hours": 4}}`
		req := httptest.NewRequest(http.MethodPost, "/v1/orgs/"+orgID+"/session-lifecycles", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", orgID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: viewerUserID, Email: viewerEmail}))

		rr := httptest.NewRecorder()
		NewSessionHandlers(nil, pool).CreateSessionLifecycle(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
		}
	})
}

func TestGetSessionLifecycle(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("pool.Ping() error = %v", err)
	}

	orgID := uuid.NewString()
	userID := uuid.NewString()
	orgMembershipID := uuid.NewString()
	email := "lc-get-test-" + strings.ToLower(uuid.NewString()) + "@example.com"
	orgSlug := "lc-get-test-org-" + strings.ToLower(uuid.NewString())
	lcID := uuid.NewString()

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM session_lifecycles WHERE id = $1`, lcID)
		_, _ = pool.Exec(ctx, `DELETE FROM orgs WHERE id = $1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	if _, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, $2, $3)`, orgID, "LC Get Test Org", orgSlug); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, $3)`, userID, email, "LC Get Test User"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO org_memberships (id, user_id, org_id, role) VALUES ($1, $2, $3, 'admin')`, orgMembershipID, userID, orgID); err != nil {
		t.Fatalf("insert org membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO session_lifecycles (id, org_id, name, slug, is_default, config) VALUES ($1, $2, $3, $4, false, '{"ttl_hours": 4}')`, lcID, orgID, "Get Test LC", "get-test"); err != nil {
		t.Fatalf("insert lifecycle: %v", err)
	}

	t.Run("GetSessionLifecycle returns org-scoped lifecycle", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/session-lifecycles/"+lcID, nil)
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", lcID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: userID, Email: email}))

		rr := httptest.NewRecorder()
		NewSessionHandlers(nil, pool).GetSessionLifecycle(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}

		var lc struct {
			ID   string `json:"id"`
			Slug string `json:"slug"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &lc); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if lc.ID != lcID {
			t.Fatalf("id = %q, want %q", lc.ID, lcID)
		}
	})

	t.Run("GetSessionLifecycle returns global lifecycle without auth", func(t *testing.T) {
		var globalID string
		if err := pool.QueryRow(ctx, `SELECT id FROM session_lifecycles WHERE org_id IS NULL LIMIT 1`).Scan(&globalID); err != nil {
			t.Skip("no global lifecycles exist")
		}

		req := httptest.NewRequest(http.MethodGet, "/v1/session-lifecycles/"+globalID, nil)
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", globalID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: userID, Email: email}))

		rr := httptest.NewRecorder()
		NewSessionHandlers(nil, pool).GetSessionLifecycle(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}
	})

	t.Run("GetSessionLifecycle returns 404 for unknown id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/session-lifecycles/"+uuid.NewString(), nil)
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", uuid.NewString())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: userID, Email: email}))

		rr := httptest.NewRecorder()
		NewSessionHandlers(nil, pool).GetSessionLifecycle(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusNotFound, rr.Body.String())
		}
	})
}

func TestUpdateAndDeleteSessionLifecycle(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("pool.Ping() error = %v", err)
	}

	orgID := uuid.NewString()
	userID := uuid.NewString()
	orgMembershipID := uuid.NewString()
	email := "lc-ud-test-" + strings.ToLower(uuid.NewString()) + "@example.com"
	orgSlug := "lc-ud-test-org-" + strings.ToLower(uuid.NewString())
	lcID := uuid.NewString()

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM session_lifecycles WHERE id = $1`, lcID)
		_, _ = pool.Exec(ctx, `DELETE FROM orgs WHERE id = $1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	if _, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, $2, $3)`, orgID, "LC UD Test Org", orgSlug); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, $3)`, userID, email, "LC UD Test User"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO org_memberships (id, user_id, org_id, role) VALUES ($1, $2, $3, 'admin')`, orgMembershipID, userID, orgID); err != nil {
		t.Fatalf("insert org membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO session_lifecycles (id, org_id, name, slug, is_default, config) VALUES ($1, $2, $3, $4, false, '{"ttl_hours": 4}')`, lcID, orgID, "UD Test LC", "ud-test"); err != nil {
		t.Fatalf("insert lifecycle: %v", err)
	}

	t.Run("UpdateSessionLifecycle updates org-scoped lifecycle", func(t *testing.T) {
		body := `{"name": "Updated LC", "config": {"ttl_hours": 12}}`
		req := httptest.NewRequest(http.MethodPatch, "/v1/session-lifecycles/"+lcID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", lcID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: userID, Email: email}))

		rr := httptest.NewRecorder()
		NewSessionHandlers(nil, pool).UpdateSessionLifecycle(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}

		var lc struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &lc); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if lc.Name != "Updated LC" {
			t.Fatalf("name = %q, want %q", lc.Name, "Updated LC")
		}
	})

	t.Run("UpdateSessionLifecycle rejects member role", func(t *testing.T) {
		viewerUserID := uuid.NewString()
		viewerEmail := "lc-ud-viewer-" + strings.ToLower(uuid.NewString()) + "@example.com"
		viewerMembershipID := uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, $3)`, viewerUserID, viewerEmail, "LC UD Viewer"); err != nil {
			t.Fatalf("insert viewer user: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO org_memberships (id, user_id, org_id, role) VALUES ($1, $2, $3, 'member')`, viewerMembershipID, viewerUserID, orgID); err != nil {
			t.Fatalf("insert viewer membership: %v", err)
		}
		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM org_memberships WHERE user_id = $1`, viewerUserID)
			_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, viewerUserID)
		})

		body := `{"name": "Should Fail"}`
		req := httptest.NewRequest(http.MethodPatch, "/v1/session-lifecycles/"+lcID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", lcID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: viewerUserID, Email: viewerEmail}))

		rr := httptest.NewRecorder()
		NewSessionHandlers(nil, pool).UpdateSessionLifecycle(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
		}
	})

	t.Run("DeleteSessionLifecycle deletes org-scoped lifecycle", func(t *testing.T) {
		delLcID := uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO session_lifecycles (id, org_id, name, slug, is_default, config) VALUES ($1, $2, $3, $4, false, '{"ttl_hours": 4}')`, delLcID, orgID, "To Delete", "to-delete-"+strings.ToLower(uuid.NewString()[:8])); err != nil {
			t.Fatalf("insert lifecycle to delete: %v", err)
		}
		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM session_lifecycles WHERE id = $1`, delLcID)
		})

		req := httptest.NewRequest(http.MethodDelete, "/v1/session-lifecycles/"+delLcID, nil)
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", delLcID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: userID, Email: email}))

		rr := httptest.NewRecorder()
		NewSessionHandlers(nil, pool).DeleteSessionLifecycle(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusNoContent, rr.Body.String())
		}
	})

	t.Run("DeleteSessionLifecycle requires admin role", func(t *testing.T) {
		delLcID := uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO session_lifecycles (id, org_id, name, slug, is_default, config) VALUES ($1, $2, $3, $4, false, '{"ttl_hours": 4}')`, delLcID, orgID, "Should Not Delete", "should-not-delete-"+strings.ToLower(uuid.NewString()[:8])); err != nil {
			t.Fatalf("insert lifecycle: %v", err)
		}
		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM session_lifecycles WHERE id = $1`, delLcID)
		})

		viewerUserID := uuid.NewString()
		viewerEmail := "lc-del-viewer-" + strings.ToLower(uuid.NewString()) + "@example.com"
		viewerMembershipID := uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, $3)`, viewerUserID, viewerEmail, "LC Del Viewer"); err != nil {
			t.Fatalf("insert viewer user: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO org_memberships (id, user_id, org_id, role) VALUES ($1, $2, $3, 'member')`, viewerMembershipID, viewerUserID, orgID); err != nil {
			t.Fatalf("insert viewer membership: %v", err)
		}
		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM org_memberships WHERE user_id = $1`, viewerUserID)
			_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, viewerUserID)
		})

		req := httptest.NewRequest(http.MethodDelete, "/v1/session-lifecycles/"+delLcID, nil)
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", delLcID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: viewerUserID, Email: viewerEmail}))

		rr := httptest.NewRecorder()
		NewSessionHandlers(nil, pool).DeleteSessionLifecycle(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
		}
	})
}

func TestGlobalSessionLifecyclesCannotBeModified(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("pool.Ping() error = %v", err)
	}

	var globalID string
	if err := pool.QueryRow(ctx, `SELECT id FROM session_lifecycles WHERE org_id IS NULL LIMIT 1`).Scan(&globalID); err != nil {
		t.Skip("no global lifecycles exist")
	}

	orgID := uuid.NewString()
	userID := uuid.NewString()
	orgMembershipID := uuid.NewString()
	email := "global-lc-test-" + strings.ToLower(uuid.NewString()) + "@example.com"
	orgSlug := "global-lc-test-org-" + strings.ToLower(uuid.NewString())

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM orgs WHERE id = $1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	if _, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, $2, $3)`, orgID, "Global LC Test Org", orgSlug); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, $3)`, userID, email, "Global LC Test User"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO org_memberships (id, user_id, org_id, role) VALUES ($1, $2, $3, 'admin')`, orgMembershipID, userID, orgID); err != nil {
		t.Fatalf("insert org membership: %v", err)
	}

	t.Run("UpdateSessionLifecycle rejects global lifecycle", func(t *testing.T) {
		body := `{"name": "Modified Global"}`
		req := httptest.NewRequest(http.MethodPatch, "/v1/session-lifecycles/"+globalID, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", globalID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: userID, Email: email}))

		rr := httptest.NewRecorder()
		NewSessionHandlers(nil, pool).UpdateSessionLifecycle(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
		}
	})

	t.Run("DeleteSessionLifecycle rejects global lifecycle", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/v1/session-lifecycles/"+globalID, nil)
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", globalID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: userID, Email: email}))

		rr := httptest.NewRecorder()
		NewSessionHandlers(nil, pool).DeleteSessionLifecycle(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusForbidden, rr.Body.String())
		}
	})
}
