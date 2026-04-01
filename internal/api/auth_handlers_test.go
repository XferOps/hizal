package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{name: "too short", password: "short7", wantErr: true},
		{name: "minimum length", password: "12345678"},
		{name: "maximum length", password: strings.Repeat("a", maxPasswordLength)},
		{name: "too long", password: strings.Repeat("a", maxPasswordLength+1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePassword(tt.password)
			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRegisterRejectsWeakPassword(t *testing.T) {
	body := `{"email":"test@example.com","name":"Test User","password":"short"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	NewAuthHandlers(nil, nil).Register(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "INVALID_PASSWORD") {
		t.Fatalf("expected INVALID_PASSWORD response, got %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "between 8 and 128 characters") {
		t.Fatalf("expected password guidance, got %s", rr.Body.String())
	}
}

func TestRegisterUserRejectsWeakPassword(t *testing.T) {
	_, _, err := NewAuthHandlers(nil, nil).registerUser(context.Background(), "test@example.com", "short", "Test User")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var validationErr *passwordValidationError
	if !strings.Contains(err.Error(), "between 8 and 128 characters") || !errors.As(err, &validationErr) {
		t.Fatalf("expected password validation error, got %v", err)
	}
}
