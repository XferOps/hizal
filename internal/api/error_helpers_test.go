package api

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestWriteInternalErrorSanitizesGenericFailures(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/context", nil)
	req = req.WithContext(context.WithValue(req.Context(), chiMiddleware.RequestIDKey, "req-123"))
	rr := httptest.NewRecorder()

	var logs bytes.Buffer
	prevWriter := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(prevWriter)

	writeInternalError(req, rr, "DB_ERROR", errors.New("relation \"context_chunks\" does not exist"))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	body := rr.Body.String()
	if strings.Contains(body, "context_chunks") {
		t.Fatalf("response leaked raw database error: %s", body)
	}
	if !strings.Contains(body, "INTERNAL_ERROR") || !strings.Contains(body, internalErrorMessage) {
		t.Fatalf("unexpected response body: %s", body)
	}
	if !strings.Contains(logs.String(), "request_id=req-123") {
		t.Fatalf("expected request id in logs, got %q", logs.String())
	}
	if !strings.Contains(logs.String(), "context_chunks") {
		t.Fatalf("expected raw error in logs, got %q", logs.String())
	}
}

func TestWriteInternalErrorMapsKnownUniqueViolations(t *testing.T) {
	t.Run("email conflict", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", nil)
		rr := httptest.NewRecorder()

		writeInternalError(req, rr, "DB_ERROR", &pgconn.PgError{
			Code:           "23505",
			ConstraintName: "users_email_key",
			Message:        "duplicate key value violates unique constraint \"users_email_key\"",
		})

		if rr.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusConflict)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "EMAIL_TAKEN") {
			t.Fatalf("expected EMAIL_TAKEN response, got %s", body)
		}
	})

	t.Run("query key conflict", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/context", nil)
		rr := httptest.NewRecorder()

		writeInternalError(req, rr, "DB_ERROR", &pgconn.PgError{
			Code:           "23505",
			ConstraintName: "context_chunks_scope_query_key_key",
			Detail:         "Key (query_key)=(spec-hizal-144) already exists.",
			Message:        "duplicate key value violates unique constraint \"context_chunks_scope_query_key_key\"",
		})

		if rr.Code != http.StatusConflict {
			t.Fatalf("status = %d, want %d", rr.Code, http.StatusConflict)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "QUERY_KEY_EXISTS") {
			t.Fatalf("expected QUERY_KEY_EXISTS response, got %s", body)
		}
	})
}
