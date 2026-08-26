package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
		wantOK bool
	}{
		{name: "a bearer token", header: "Bearer abc.def.ghi", want: "abc.def.ghi", wantOK: true},
		// RFC 6750 names the scheme case-insensitively, and clients differ.
		{name: "lowercase scheme", header: "bearer abc", want: "abc", wantOK: true},
		{name: "mixed case scheme", header: "BeArEr abc", want: "abc", wantOK: true},
		{name: "no header", header: "", wantOK: false},
		{name: "another scheme", header: "Basic dXNlcjpwYXNz", wantOK: false},
		{name: "scheme with no token", header: "Bearer", wantOK: false},
		{name: "scheme with only spaces", header: "Bearer    ", wantOK: false},
		// A token is a single opaque string; a second field means the caller sent
		// something other than what we can verify.
		{name: "two tokens", header: "Bearer abc def", wantOK: false},
		{name: "bare token with no scheme", header: "abc.def.ghi", wantOK: false},
		{name: "leading and trailing space", header: "  Bearer abc  ", want: "abc", wantOK: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := bearerToken(test.header)
			if ok != test.wantOK {
				t.Fatalf("bearerToken(%q) ok = %v, want %v", test.header, ok, test.wantOK)
			}
			if got != test.want {
				t.Errorf("bearerToken(%q) = %q, want %q", test.header, got, test.want)
			}
		})
	}
}

// fakeVerifier stands in for the identity provider so the middleware's behavior
// can be tested without one.
type fakeVerifier struct {
	subject Subject
	err     error
	seen    string
}

func (f *fakeVerifier) Verify(_ context.Context, rawToken string) (Subject, error) {
	f.seen = rawToken
	return f.subject, f.err
}

func TestMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		header         string
		verifyErr      error
		wantStatus     int
		wantHandlerRun bool
	}{
		{
			name:           "a valid token reaches the handler",
			header:         "Bearer good",
			wantStatus:     http.StatusOK,
			wantHandlerRun: true,
		},
		{
			name:       "no credentials",
			header:     "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "wrong scheme",
			header:     "Basic dXNlcjpwYXNz",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "a token the provider rejects",
			header:     "Bearer bad",
			verifyErr:  errors.New("signature mismatch"),
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			verifier := &fakeVerifier{subject: Subject{ID: "user-1"}, err: test.verifyErr}

			handlerRan := false
			protected := Middleware(verifier)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				handlerRan = true
				subject, ok := SubjectFrom(r.Context())
				if !ok {
					t.Error("handler ran without a subject on the context")
				}
				if subject.ID != "user-1" {
					t.Errorf("subject.ID = %q, want %q", subject.ID, "user-1")
				}
			}))

			request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
			if test.header != "" {
				request.Header.Set("Authorization", test.header)
			}
			recorder := httptest.NewRecorder()
			protected.ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			if handlerRan != test.wantHandlerRun {
				t.Errorf("handler ran = %v, want %v", handlerRan, test.wantHandlerRun)
			}
			// RFC 6750: a 401 has to say how to authenticate, or a client has
			// nothing to act on.
			if recorder.Code == http.StatusUnauthorized {
				if got := recorder.Header().Get("WWW-Authenticate"); got == "" {
					t.Error("401 response carries no WWW-Authenticate header")
				}
			}
		})
	}
}

// The middleware must not leak why verification failed. "signature mismatch" and
// "expired 3 seconds ago" are useful to an operator reading logs and useful to an
// attacker reading responses.
func TestMiddlewareDoesNotEchoTheVerifierError(t *testing.T) {
	verifier := &fakeVerifier{err: errors.New("token expired at 2026-01-01, signed by kid abc")}
	protected := Middleware(verifier)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/whatever", nil)
	request.Header.Set("Authorization", "Bearer expired")
	recorder := httptest.NewRecorder()
	protected.ServeHTTP(recorder, request)

	body := recorder.Body.String()
	for _, leak := range []string{"expired at 2026-01-01", "kid abc"} {
		if strings.Contains(body, leak) {
			t.Errorf("response body leaks verifier detail %q: %s", leak, body)
		}
	}
}
