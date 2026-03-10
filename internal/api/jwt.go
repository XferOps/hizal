package api

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type jwtContextKey struct{}

type JWTClaims struct {
	UserID string `json:"sub"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

type JWTUser struct {
	ID    string
	Email string
}

func jwtSecret() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		s = "dev-secret-change-me-in-production"
	}
	return []byte(s)
}

func SignJWT(userID, email string) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret())
}

func ParseJWT(tokenStr string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &JWTClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return jwtSecret(), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}

func withJWTUser(ctx context.Context, u JWTUser) context.Context {
	return context.WithValue(ctx, jwtContextKey{}, u)
}

func JWTUserFrom(ctx context.Context) (JWTUser, bool) {
	u, ok := ctx.Value(jwtContextKey{}).(JWTUser)
	return u, ok
}

// JWTAuth is middleware that validates JWT Bearer tokens.
func JWTAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "missing or invalid Authorization header")
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			claims, err := ParseJWT(tokenStr)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "AUTH_INVALID", "invalid or expired token")
				return
			}
			user := JWTUser{ID: claims.UserID, Email: claims.Email}
			next.ServeHTTP(w, r.WithContext(withJWTUser(r.Context(), user)))
		})
	}
}
