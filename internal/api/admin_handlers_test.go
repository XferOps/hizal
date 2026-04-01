package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAdminListOrgsAllowsMissingOwnerEmail(t *testing.T) {
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
	orgSlug := "admin-org-" + strings.ToLower(uuid.NewString())

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM orgs WHERE id = $1`, orgID)
	})

	if _, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, $2, $3)`, orgID, "Ownerless Org", orgSlug); err != nil {
		t.Fatalf("insert org: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/orgs", nil)
	rr := httptest.NewRecorder()

	NewAdminHandlers(pool).ListOrgs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var orgs []struct {
		ID         string  `json:"id"`
		OwnerEmail *string `json:"owner_email"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &orgs); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	for _, org := range orgs {
		if org.ID == orgID {
			if org.OwnerEmail != nil {
				t.Fatalf("owner_email = %v, want nil", *org.OwnerEmail)
			}
			return
		}
	}

	t.Fatalf("org %s not found in response", orgID)
}

func TestAdminListKeysAllowsMissingUserAndOrg(t *testing.T) {
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
	ownerID := uuid.NewString()
	agentID := uuid.NewString()
	keyID := uuid.NewString()
	orgSlug := "admin-key-org-" + strings.ToLower(uuid.NewString())
	ownerEmail := "admin-key-owner-" + strings.ToLower(uuid.NewString()) + "@example.com"
	agentSlug := "admin-agent-" + strings.ToLower(uuid.NewString())
	keyHash := "hash-" + strings.ToLower(uuid.NewString())

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM orgs WHERE id = $1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, ownerID)
	})

	if _, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, $2, $3)`, orgID, "Admin Key Org", orgSlug); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, email, name) VALUES ($1, $2, $3)`, ownerID, ownerEmail, "Admin Key Owner"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO agents (id, org_id, owner_id, name, slug) VALUES ($1, $2, $3, $4, $5)`, agentID, orgID, ownerID, "Admin Agent", agentSlug); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO api_keys (id, key_hash, name, owner_type, agent_id, org_id)
		VALUES ($1, $2, $3, 'AGENT', $4, NULL)
	`, keyID, keyHash, "Agent Key", agentID); err != nil {
		t.Fatalf("insert api key: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/keys", nil)
	rr := httptest.NewRecorder()

	NewAdminHandlers(pool).ListKeys(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var keys []struct {
		ID         string  `json:"id"`
		UserEmail  *string `json:"user_email"`
		OrgID      *string `json:"org_id"`
		OrgName    *string `json:"org_name"`
		LastUsedAt *string `json:"last_used_at"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &keys); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	for _, key := range keys {
		if key.ID == keyID {
			if key.UserEmail != nil {
				t.Fatalf("user_email = %v, want nil", *key.UserEmail)
			}
			if key.OrgID != nil {
				t.Fatalf("org_id = %v, want nil", *key.OrgID)
			}
			if key.OrgName != nil {
				t.Fatalf("org_name = %v, want nil", *key.OrgName)
			}
			if key.LastUsedAt != nil {
				t.Fatalf("last_used_at = %v, want nil", *key.LastUsedAt)
			}
			return
		}
	}

	t.Fatalf("api key %s not found in response", keyID)
}
