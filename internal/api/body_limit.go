package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// BodyLimit caps request bodies for routes that accept JSON payloads.
func BodyLimit(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func decodeJSONBody(r *http.Request, dst interface{}) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func writeJSONDecodeError(w http.ResponseWriter, err error, fallbackMessage string) {
	if isRequestBodyTooLarge(err) {
		writeBodyTooLarge(w)
		return
	}
	if fallbackMessage == "" && err != nil {
		fallbackMessage = err.Error()
	}
	writeError(w, http.StatusBadRequest, "INVALID_BODY", fallbackMessage)
}

func writeBodyTooLarge(w http.ResponseWriter) {
	writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body exceeds the configured size limit")
}

func isRequestBodyTooLarge(err error) bool {
	if err == nil {
		return false
	}
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "request body too large")
}
