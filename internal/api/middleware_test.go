package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
