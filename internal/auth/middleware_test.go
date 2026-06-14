package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJWTMiddleware_ValidToken(t *testing.T) {
	m := NewJWTMiddleware("test-secret-key")

	// Generate a valid mock token
	token, err := m.GenerateMockToken("client_123", "premium")
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// Create request with Bearer token
	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()

	// Handler assertion check
	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID := r.Context().Value(ClientIDCtxKey).(string)
		tier := r.Context().Value(ClientTierCtxKey).(string)

		if clientID != "client_123" {
			t.Errorf("expected clientID 'client_123', got %q", clientID)
		}
		if tier != "premium" {
			t.Errorf("expected tier 'premium', got %q", tier)
		}

		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestJWTMiddleware_AnonymousFallback(t *testing.T) {
	m := NewJWTMiddleware("test-secret-key")

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	rr := httptest.NewRecorder()

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID := r.Context().Value(ClientIDCtxKey).(string)
		tier := r.Context().Value(ClientTierCtxKey).(string)

		if clientID != "anonymous" {
			t.Errorf("expected clientID 'anonymous', got %q", clientID)
		}
		if tier != "anonymous" {
			t.Errorf("expected tier 'anonymous', got %q", tier)
		}

		w.WriteHeader(http.StatusOK)
	}))

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

func TestJWTMiddleware_InvalidToken(t *testing.T) {
	m := NewJWTMiddleware("test-secret-key")

	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-string")

	rr := httptest.NewRecorder()

	handler := m.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not have been called")
	}))

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["error"] != "invalid_token" {
		t.Errorf("expected error code 'invalid_token', got %q", resp["error"])
	}
}
