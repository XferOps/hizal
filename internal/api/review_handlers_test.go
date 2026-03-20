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

func TestReviewInbox(t *testing.T) {
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
	orgSlug := "review-inbox-" + strings.ToLower(uuid.NewString())
	projectSlug := "proj-" + strings.ToLower(uuid.NewString())

	insert := func(query string, args ...interface{}) {
		if _, err := pool.Exec(ctx, query, args...); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}

	cleanup := func(ids ...string) {
		for _, id := range ids {
			pool.Exec(ctx, `DELETE FROM context_reviews WHERE chunk_id = $1`, id)
			pool.Exec(ctx, `DELETE FROM context_chunks WHERE id = $1`, id)
		}
	}

	insert(`INSERT INTO orgs (id, name, slug) VALUES ($1, $2, $3)`, orgID, "Review Inbox Test Org", orgSlug)
	insert(`INSERT INTO projects (id, org_id, name, slug) VALUES ($1, $2, $3, $4)`, projectID, orgID, "Test Project", projectSlug)

	chunkNeedsUpdate := uuid.NewString()
	chunkLowUsefulness := uuid.NewString()
	chunkLowCorrectness := uuid.NewString()
	chunkOldReview := uuid.NewString()
	chunkRecentGoodReview := uuid.NewString()
	chunkNoReview := uuid.NewString()
	chunkGoodRecent := uuid.NewString()

	insert(`INSERT INTO context_chunks (id, org_id, project_id, query_key, title, scope, chunk_type) VALUES ($1, $2, $3, $4, $5, 'PROJECT', 'KNOWLEDGE')`,
		chunkNeedsUpdate, orgID, projectID, "key-flagged", "Flagged chunk")
	insert(`INSERT INTO context_chunks (id, org_id, project_id, query_key, title, scope, chunk_type) VALUES ($1, $2, $3, $4, $5, 'PROJECT', 'CONVENTION')`,
		chunkLowUsefulness, orgID, projectID, "key-low-u", "Low usefulness chunk")
	insert(`INSERT INTO context_chunks (id, org_id, project_id, query_key, title, scope, chunk_type) VALUES ($1, $2, $3, $4, $5, 'ORG', 'DECISION')`,
		chunkLowCorrectness, orgID, nil, "key-low-c", "Low correctness chunk")
	insert(`INSERT INTO context_chunks (id, org_id, project_id, query_key, title, scope, chunk_type) VALUES ($1, $2, $3, $4, $5, 'PROJECT', 'KNOWLEDGE')`,
		chunkOldReview, orgID, projectID, "key-old", "Old review chunk")
	insert(`INSERT INTO context_chunks (id, org_id, project_id, query_key, title, scope, chunk_type) VALUES ($1, $2, $3, $4, $5, 'PROJECT', 'KNOWLEDGE')`,
		chunkRecentGoodReview, orgID, projectID, "key-recent-good", "Recent good review")
	insert(`INSERT INTO context_chunks (id, org_id, project_id, query_key, title, scope, chunk_type) VALUES ($1, $2, $3, $4, $5, 'PROJECT', 'KNOWLEDGE')`,
		chunkNoReview, orgID, projectID, "key-no-review", "No review chunk")
	insert(`INSERT INTO context_chunks (id, org_id, project_id, query_key, title, scope, chunk_type) VALUES ($1, $2, $3, $4, $5, 'PROJECT', 'KNOWLEDGE')`,
		chunkGoodRecent, orgID, projectID, "key-good", "Good recent chunk")

	for _, id := range []string{chunkNeedsUpdate, chunkLowUsefulness, chunkLowCorrectness, chunkOldReview, chunkRecentGoodReview, chunkNoReview, chunkGoodRecent} {
		t.Cleanup(func() { cleanup(id) })
	}

	insert(`INSERT INTO context_reviews (chunk_id, action, usefulness, correctness, usefulness_note, created_at) VALUES ($1, 'needs_update', 5, 5, 'outdated info', NOW())`,
		chunkNeedsUpdate)
	insert(`INSERT INTO context_reviews (chunk_id, action, usefulness, correctness, correctness_note, created_at) VALUES ($1, 'approved', 1, 5, 'confusing', NOW())`,
		chunkLowUsefulness)
	insert(`INSERT INTO context_reviews (chunk_id, action, usefulness, correctness, correctness_note, created_at) VALUES ($1, 'approved', 5, 2, 'factually wrong', NOW())`,
		chunkLowCorrectness)
	insert(`INSERT INTO context_reviews (chunk_id, action, usefulness, correctness, created_at) VALUES ($1, 'approved', 5, 5, NOW() - INTERVAL '45 days')`,
		chunkOldReview)
	insert(`INSERT INTO context_reviews (chunk_id, action, usefulness, correctness, created_at) VALUES ($1, 'approved', 5, 5, NOW() - INTERVAL '5 days')`,
		chunkRecentGoodReview)
	insert(`INSERT INTO context_reviews (chunk_id, action, usefulness, correctness, created_at) VALUES ($1, 'approved', 5, 5, NOW() - INTERVAL '10 days')`,
		chunkGoodRecent)

	h := NewReviewHandlers(pool)

	makeRequest := func(id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/orgs/"+id+"/review-inbox", nil)
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", id)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		req = req.WithContext(withClaims(req.Context(), AuthClaims{OrgID: id}))

		rr := httptest.NewRecorder()
		h.ReviewInbox(rr, req)
		return rr
	}

	t.Run("Returns 401 without auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/orgs/"+orgID+"/review-inbox", nil)
		routeCtx := chi.NewRouteContext()
		routeCtx.URLParams.Add("id", orgID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
		rr := httptest.NewRecorder()
		h.ReviewInbox(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
		}
	})

	t.Run("Returns chunks needing review", func(t *testing.T) {
		rr := makeRequest(orgID)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}

		var resp ReviewInboxResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		ids := make(map[string]bool)
		for _, c := range resp.Chunks {
			ids[c.ID] = true
		}

		if !ids[chunkNeedsUpdate] {
			t.Errorf("missing chunk with needs_update action")
		}
		if !ids[chunkLowUsefulness] {
			t.Errorf("missing chunk with low usefulness")
		}
		if !ids[chunkLowCorrectness] {
			t.Errorf("missing chunk with low correctness")
		}
		if !ids[chunkOldReview] {
			t.Errorf("missing chunk with old review (45 days)")
		}
		if !ids[chunkNoReview] {
			t.Errorf("missing chunk with no reviews")
		}

		if ids[chunkRecentGoodReview] {
			t.Errorf("chunkRecentGoodReview should NOT be in inbox (reviewed 5 days ago, no flags)")
		}
		if ids[chunkGoodRecent] {
			t.Errorf("chunkGoodRecent should NOT be in inbox (reviewed 10 days ago, no flags)")
		}
	})

	t.Run("Response has correct fields", func(t *testing.T) {
		rr := makeRequest(orgID)
		var resp ReviewInboxResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		for _, c := range resp.Chunks {
			if c.ID == "" {
				t.Error("chunk id should not be empty")
			}
			if c.QueryKey == "" {
				t.Error("query_key should not be empty")
			}
			if c.Title == "" {
				t.Error("title should not be empty")
			}
			if c.Scope == "" {
				t.Error("scope should not be empty")
			}
			if c.ChunkType == "" {
				t.Error("chunk_type should not be empty")
			}
			if c.LastReviewAt == nil {
				t.Error("last_review_at should not be nil")
			}
			if c.Freshness <= 0 || c.Freshness > 1 {
				t.Errorf("freshness = %f, want between 0 and 1", c.Freshness)
			}
		}
	})

	t.Run("Stale chunks come first", func(t *testing.T) {
		rr := makeRequest(orgID)
		var resp ReviewInboxResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		if len(resp.Chunks) == 0 {
			t.Fatal("expected chunks in inbox")
		}

		first := resp.Chunks[0]
		if first.ID != chunkNeedsUpdate && first.ID != chunkLowUsefulness && first.ID != chunkLowCorrectness {
			t.Errorf("first chunk should be a flagged/low-score chunk, got %s", first.ID)
		}
	})

	t.Run("Total reflects actual count", func(t *testing.T) {
		rr := makeRequest(orgID)
		var resp ReviewInboxResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		if resp.Total != len(resp.Chunks) {
			t.Errorf("total = %d, want %d", resp.Total, len(resp.Chunks))
		}
	})

	t.Run("Returns empty array for org with no reviewable chunks", func(t *testing.T) {
		emptyOrgID := uuid.NewString()
		insert(`INSERT INTO orgs (id, name, slug) VALUES ($1, $2, $3)`, emptyOrgID, "Empty Org", "empty-"+uuid.NewString())
		t.Cleanup(func() {
			pool.Exec(ctx, `DELETE FROM orgs WHERE id = $1`, emptyOrgID)
		})

		rr := makeRequest(emptyOrgID)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
		}

		var resp ReviewInboxResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		if resp.Total != 0 {
			t.Errorf("total = %d, want 0", resp.Total)
		}
		if len(resp.Chunks) != 0 {
			t.Errorf("chunks = %v, want empty", resp.Chunks)
		}
	})

	t.Run("Project-scoped chunks belong to org", func(t *testing.T) {
		rr := makeRequest(orgID)
		var resp ReviewInboxResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		projChunks := 0
		for _, c := range resp.Chunks {
			if c.ProjectID != nil && *c.ProjectID == projectID {
				projChunks++
				if c.ProjectName == nil || *c.ProjectName != "Test Project" {
					t.Errorf("project_name mismatch for chunk %s", c.ID)
				}
			}
		}
		if projChunks == 0 {
			t.Errorf("expected some chunks from project %s", projectID)
		}
	})

	t.Run("Stale signals include action and note", func(t *testing.T) {
		rr := makeRequest(orgID)
		var resp ReviewInboxResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		for _, c := range resp.Chunks {
			if c.ID == chunkNeedsUpdate {
				if len(c.StaleSignals) == 0 {
					t.Errorf("chunkNeedsUpdate should have stale signals")
				}
				found := false
				for _, s := range c.StaleSignals {
					if s.Action == "needs_update" {
						found = true
					}
				}
				if !found {
					t.Errorf("chunkNeedsUpdate should have a needs_update signal")
				}
			}
		}
	})

	t.Run("Freshness is computed correctly", func(t *testing.T) {
		rr := makeRequest(orgID)
		var resp ReviewInboxResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		for _, c := range resp.Chunks {
			if c.ID == chunkNeedsUpdate || c.ID == chunkLowUsefulness || c.ID == chunkLowCorrectness {
				if c.Freshness != 1.0 {
					t.Errorf("freshness = %f for recently-reviewed chunk, want 1.0", c.Freshness)
				}
			}
			if c.ID == chunkOldReview {
				if c.Freshness >= 1.0 {
					t.Errorf("freshness = %f for old-reviewed chunk, want < 1.0", c.Freshness)
				}
			}
		}
	})

	t.Run("Days since review is positive", func(t *testing.T) {
		rr := makeRequest(orgID)
		var resp ReviewInboxResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		for _, c := range resp.Chunks {
			if c.DaysSinceReview < 0 {
				t.Errorf("days_since_review = %d, want >= 0", c.DaysSinceReview)
			}
		}
	})

	t.Cleanup(func() {
		pool.Exec(ctx, `DELETE FROM context_chunks WHERE org_id = $1 OR project_id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, projectID)
		pool.Exec(ctx, `DELETE FROM orgs WHERE id = $1`, orgID)
	})

}
