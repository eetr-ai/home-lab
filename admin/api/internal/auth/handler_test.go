package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWhoami(t *testing.T) {
	mux := http.NewServeMux()
	NewHandler().Register(mux)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whoami", nil)
	request = request.WithContext(context.WithValue(
		request.Context(), subjectKey, Subject{ID: "user-abc", Email: "operator@example.invalid"}))

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	var body WhoamiResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Subject != "user-abc" {
		t.Errorf("subject = %q, want %q", body.Subject, "user-abc")
	}
	if body.Email != "operator@example.invalid" {
		t.Errorf("email = %q, want the email claim", body.Email)
	}
}

// Without Middleware in front there is no subject, and the handler has to say so
// rather than panicking on a missing context value.
func TestWhoamiWithoutAVerifiedCaller(t *testing.T) {
	mux := http.NewServeMux()
	NewHandler().Register(mux)

	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whoami", nil))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}
