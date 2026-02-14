package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireInternalAuth_MissingHeader(t *testing.T) {
	systemSecret := "test-secret-61"
	middleware := RequireInternalAuth(systemSecret)

	// Dummy handler that should NOT be called
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not have been called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/internal/foo", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
	}
	if body := rr.Body.String(); body != "Missing authorization header\n" {
		t.Errorf("got body %q, want %q", body, "Missing authorization header\n")
	}
}

func TestRequireInternalAuth_InvalidHeaderFormat(t *testing.T) {
	systemSecret := "test-secret-61"
	middleware := RequireInternalAuth(systemSecret)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not have been called")
	}))

	invalidHeaders := []string{
		"Basic test-secret-61",
		"Bearer",
		"Token test-secret-61",
		"test-secret-61",
		"Bearer  test-secret-61", // Double space
	}

	for _, h := range invalidHeaders {
		req := httptest.NewRequest(http.MethodGet, "/internal/foo", nil)
		req.Header.Set("Authorization", h)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("header %q: got status %d, want %d", h, rr.Code, http.StatusUnauthorized)
		}
	}
}

func TestRequireInternalAuth_InvalidToken(t *testing.T) {
	systemSecret := "correct-secret"
	middleware := RequireInternalAuth(systemSecret)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not have been called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/internal/foo", nil)
	req.Header.Set("Authorization", "Bearer wrong-secret")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}

func TestRequireInternalAuth_Success(t *testing.T) {
	systemSecret := "super-secret-system-key"
	middleware := RequireInternalAuth(systemSecret)

	called := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/internal/foo", nil)
	req.Header.Set("Authorization", "Bearer "+systemSecret)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}
	if !called {
		t.Error("Next handler was not called")
	}
}
