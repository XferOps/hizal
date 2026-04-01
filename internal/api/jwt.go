package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
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

var jwtStartupFatalf = log.Fatalf

func jwtSecretValidationError() error {
	env := os.Getenv("ENV")
	if env == "development" {
		return nil
	}
	if os.Getenv("JWT_SECRET") != "" {
		return nil
	}
	if env == "" {
		env = "unset"
	}
	return fmt.Errorf("JWT_SECRET must be set when ENV is %q", env)
}

func requireJWTSecretForStartup() {
	if err := jwtSecretValidationError(); err != nil {
		jwtStartupFatalf("invalid JWT configuration: %v", err)
	}
}

func jwtSecret() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		s = "dev-secret-change-me-in-production"
	}
	return []byte(s)
}

func SignJWT(userID, email string) (string, error) {
	return SignJWTWithExpiry(userID, email, 15*time.Minute)
}

func SignJWTWithExpiry(userID, email string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
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

// GenerateRefreshToken generates a cryptographically secure refresh token.
func GenerateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashRefreshToken hashes a refresh token using SHA-256 for secure storage.
func HashRefreshToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
