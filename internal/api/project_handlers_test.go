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

func TestListProjectsIncludesAccessMetadata(t *testing.T) {
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
	accessibleProjectID := uuid.NewString()
	inaccessibleProjectID := uuid.NewString()
	projectMembershipID := uuid.NewString()
	email := "project-list-" + strings.ToLower(uuid.NewString()) + "@example.com"
	orgSlug := "project-list-org-" + strings.ToLower(uuid.NewString())
	accessibleSlug := "accessible-" + strings.ToLower(uuid.NewString())
	inaccessibleSlug := "inaccessible-" + strings.ToLower(uuid.NewString())
	accessibleDescription := "Primary app context"
	inaccessibleDescription := "Internal admin tooling"

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM orgs WHERE id = $1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	if _, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, $2, $3)`, orgID, "Project List Org", orgSlug); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, $3)`, userID, email, "Project List User"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO org_memberships (id, user_id, org_id, role) VALUES ($1, $2, $3, 'member')`, orgMembershipID, userID, orgID); err != nil {
		t.Fatalf("insert org membership: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, slug, description) VALUES ($1, $2, $3, $4, $5)`, accessibleProjectID, orgID, "Accessible Project", accessibleSlug, accessibleDescription); err != nil {
		t.Fatalf("insert accessible project: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, org_id, name, slug, description) VALUES ($1, $2, $3, $4, $5)`, inaccessibleProjectID, orgID, "Inaccessible Project", inaccessibleSlug, inaccessibleDescription); err != nil {
		t.Fatalf("insert inaccessible project: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO project_memberships (id, user_id, project_id, role) VALUES ($1, $2, $3, 'viewer')`, projectMembershipID, userID, accessibleProjectID); err != nil {
		t.Fatalf("insert project membership: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/orgs/"+orgID+"/projects", nil)
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("id", orgID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	req = req.WithContext(withJWTUser(req.Context(), JWTUser{ID: userID, Email: email}))

	rr := httptest.NewRecorder()
	NewProjectHandlers(pool).ListProjects(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var body struct {
		Projects []struct {
			ID            string  `json:"id"`
			Name          string  `json:"name"`
			Description   *string `json:"description"`
			IsMember      bool    `json:"is_member"`
			EffectiveRole *string `json:"effective_role"`
			CanOpen       bool    `json:"can_open"`
		} `json:"projects"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if len(body.Projects) != 2 {
		t.Fatalf("len(projects) = %d, want 2", len(body.Projects))
	}

	results := make(map[string]struct {
		Description   *string
		IsMember      bool
		EffectiveRole *string
		CanOpen       bool
	}, len(body.Projects))
	for _, project := range body.Projects {
		results[project.ID] = struct {
			Description   *string
			IsMember      bool
			EffectiveRole *string
			CanOpen       bool
		}{
			Description:   project.Description,
			IsMember:      project.IsMember,
			EffectiveRole: project.EffectiveRole,
			CanOpen:       project.CanOpen,
		}
	}

	accessible := results[accessibleProjectID]
	if !accessible.IsMember || !accessible.CanOpen {
		t.Fatalf("accessible project flags = %+v, want member/open", accessible)
	}
	if accessible.EffectiveRole == nil || *accessible.EffectiveRole != "viewer" {
		t.Fatalf("accessible effective_role = %v, want viewer", accessible.EffectiveRole)
	}
	if accessible.Description == nil || *accessible.Description != accessibleDescription {
		t.Fatalf("accessible description = %v, want %q", accessible.Description, accessibleDescription)
	}

	inaccessible := results[inaccessibleProjectID]
	if inaccessible.IsMember || inaccessible.CanOpen {
		t.Fatalf("inaccessible project flags = %+v, want not member/not open", inaccessible)
	}
	if inaccessible.EffectiveRole != nil {
		t.Fatalf("inaccessible effective_role = %v, want nil", *inaccessible.EffectiveRole)
	}
	if inaccessible.Description == nil || *inaccessible.Description != inaccessibleDescription {
		t.Fatalf("inaccessible description = %v, want %q", inaccessible.Description, inaccessibleDescription)
	}
}
