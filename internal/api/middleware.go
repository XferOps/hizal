package api

import (
	"net/http"
	"strings"
)

// APIKeyAuth is a placeholder middleware for API key authentication.
// It will validate Bearer tokens against the api_keys table.
func APIKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// TODO: validate token against api_keys table, set user/project context
		_ = strings.TrimPrefix(authHeader, "Bearer ")

		next.ServeHTTP(w, r)
	})
}
