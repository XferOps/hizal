package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestContextAuthJWTRequiresProjectScope(t *testing.T) {
	token, err := SignJWT("user-123", "agent@example.com")
	if err != nil {
		t.Fatalf("SignJWT() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/context/search?query=*", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	handler := ContextAuth(&pgxpool.Pool{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called when project scope is missing")
	}))

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestProjectIDFallsBackToQueryParam(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/context/search?project_id=project-123", nil)

	if got := projectID(req); got != "project-123" {
		t.Fatalf("projectID() = %q, want %q", got, "project-123")
	}
}

func TestContextAuthJWTRequiresProjectMembership(t *testing.T) {
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
	projectID := uuid.NewString()
	userID := uuid.NewString()
	orgMembershipID := uuid.NewString()
	email := "context-auth-" + strings.ToLower(uuid.NewString()) + "@example.com"
	slug := "context-auth-" + strings.ToLower(uuid.NewString())

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM orgs WHERE id = $1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	if _, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, $2, $3)`, orgID, "Context Auth Org", slug); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, $3)`, userID, email, "Context Auth User"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO org_memberships (id, user_id, org_id, role) VALUES ($1, $2, $3, 'member')`, orgMembershipID, userID, orgID); err != nil {
		t.Fatalf("insert org membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, slug) VALUES ($1, $2, $3, $4)`, projectID, orgID, "Restricted Project", "restricted-"+strings.ToLower(uuid.NewString())); err != nil {
		t.Fatalf("insert project: %v", err)
	}

	token, err := SignJWT(userID, email)
	if err != nil {
		t.Fatalf("SignJWT() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/context/search?query=*&project_id="+projectID, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	called := false
	handler := ContextAuth(pool)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	if called {
		t.Fatal("next handler should not be called when project membership is missing")
	}
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
	if body := rr.Body.String(); !strings.Contains(body, "not a member of this project") {
		t.Fatalf("body = %q, want membership error", body)
	}
}

func TestSkillAuthAcceptsJWTWithoutPool(t *testing.T) {
	token, err := SignJWT("user-123", "skills@example.com")
	if err != nil {
		t.Fatalf("SignJWT() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/skills", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	called := false
	handler := SkillAuth(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		user, ok := JWTUserFrom(r.Context())
		if !ok || user.ID != "user-123" {
			t.Fatalf("JWT user missing from context: %+v, ok=%v", user, ok)
		}
		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatal("next handler should be called for valid JWT")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}
