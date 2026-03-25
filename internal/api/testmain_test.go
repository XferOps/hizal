package api

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("ENV") == "" {
		_ = os.Setenv("ENV", "development")
	}
	if os.Getenv("JWT_SECRET") == "" {
		_ = os.Setenv("JWT_SECRET", "test-jwt-secret")
	}
	os.Exit(m.Run())
}
