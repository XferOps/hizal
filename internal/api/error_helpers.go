package api

import (
	"errors"
	"log"
	"net/http"
	"strings"

	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgconn"
)

const internalErrorMessage = "an internal error occurred"

func writeInternalError(r *http.Request, w http.ResponseWriter, code string, err error) {
	logRequestError(r, code, err)

	if conflictCode, conflictMessage, ok := conflictResponseForError(err); ok {
		writeError(w, http.StatusConflict, conflictCode, conflictMessage)
		return
	}

	writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", internalErrorMessage)
}

func logRequestError(r *http.Request, code string, err error) {
	if err == nil {
		return
	}

	if r == nil {
		log.Printf("api error code=%s err=%v", code, err)
		return
	}

	requestID := chiMiddleware.GetReqID(r.Context())
	if requestID == "" {
		log.Printf("api error code=%s method=%s path=%s err=%v", code, r.Method, r.URL.Path, err)
		return
	}

	log.Printf("api error code=%s request_id=%s method=%s path=%s err=%v", code, requestID, r.Method, r.URL.Path, err)
}

func conflictResponseForError(err error) (string, string, bool) {
	if !isUniqueViolation(err) {
		return "", "", false
	}

	msg := uniqueViolationText(err)
	switch {
	case strings.Contains(msg, "users_email_key"), strings.Contains(msg, " email"):
		return "EMAIL_TAKEN", "a user with that email already exists", true
	case strings.Contains(msg, "query_key"):
		return "QUERY_KEY_EXISTS", "a chunk with that query_key already exists", true
	default:
		return "", "", false
	}
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}

	if pgErr := pgError(err); pgErr != nil {
		return pgErr.Code == "23505"
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "23505") || strings.Contains(msg, "unique")
}

func uniqueViolationText(err error) string {
	parts := []string{strings.ToLower(err.Error())}
	if pgErr := pgError(err); pgErr != nil {
		parts = append(parts,
			strings.ToLower(pgErr.ConstraintName),
			strings.ToLower(pgErr.Detail),
			strings.ToLower(pgErr.Message),
		)
	}
	return strings.Join(parts, " ")
}

func pgError(err error) *pgconn.PgError {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr
	}
	return nil
}
