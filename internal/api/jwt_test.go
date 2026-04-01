package api

import (
	"fmt"
	"testing"
)

func TestJWTSecretValidationErrorAllowsDevelopmentFallback(t *testing.T) {
	t.Setenv("ENV", "development")
	t.Setenv("JWT_SECRET", "")

	if err := jwtSecretValidationError(); err != nil {
		t.Fatalf("jwtSecretValidationError() error = %v, want nil", err)
	}
}

func TestJWTSecretValidationErrorRequiresSecretOutsideDevelopment(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("JWT_SECRET", "")

	err := jwtSecretValidationError()
	if err == nil {
		t.Fatal("jwtSecretValidationError() error = nil, want error")
	}
	if got, want := err.Error(), `JWT_SECRET must be set when ENV is "production"`; got != want {
		t.Fatalf("jwtSecretValidationError() = %q, want %q", got, want)
	}
}

func TestRequireJWTSecretForStartupFatalf(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("JWT_SECRET", "")

	originalFatalf := jwtStartupFatalf
	defer func() { jwtStartupFatalf = originalFatalf }()

	var got string
	jwtStartupFatalf = func(format string, args ...interface{}) {
		got = fmt.Sprintf(format, args...)
	}

	requireJWTSecretForStartup()

	if want := `invalid JWT configuration: JWT_SECRET must be set when ENV is "production"`; got != want {
		t.Fatalf("fatal message = %q, want %q", got, want)
	}
}
