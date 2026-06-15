package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/http.TestServer"
	"net/http/httptest"
	"testing"
)

type mockValidator struct {
	shouldFail bool
	identity   *UserIdentity
}

func (m *mockValidator) Validate(ctx context.Context, token string) (*UserIdentity, error) {
	if m.shouldFail {
		return nil, ErrInvalidToken
	}
	return m.identity, nil
}

func TestAuthMiddleware(t *testing.T) {
	logger := slog.Default()
	identity := &UserIdentity{UserID: "user-123", TenantID: "tenant-abc"}
	validator := &mockValidator{identity: identity}
	mw := NewAuthMiddleware(validator, logger)

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify downstream propagation
		if r.Header.Get("Authorization") != "" {
			t.Error("Authorization header should have been stripped")
		}
		if r.Header.Get("X-User-ID") != "user-123" {
			t.Errorf("Expected X-User-ID user-123, got %s", r.Header.Get("X-User-ID"))
		}
		if _, ok := GetIdentity(r.Context()); !ok {
			t.Error("UserIdentity missing from context")
		}
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name           string
		authHeader     string
		validatorFail  bool
		expectedStatus int
	}{
		{"Missing Header", "", false, http.StatusUnauthorized},
		{"Malformed Header", "Basic 123", false, http.StatusUnauthorized},
		{"Invalid Token", "Bearer invalid-token", true, http.StatusUnauthorized},
		{"Valid Token", "Bearer valid-token", false, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator.shouldFail = tt.validatorFail
			req := httptest.NewRequest("GET", "/", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			
			rr := httptest.NewRecorder()
			mw(nextHandler).ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}
		})
	}
}