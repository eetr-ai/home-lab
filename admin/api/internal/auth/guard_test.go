package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGuardRequire(t *testing.T) {
	tests := []struct {
		name          string
		requireScopes bool
		subject       *Subject
		want          int
		reached       bool
	}{
		{
			// The compatibility rule, and the one that matters most today: the
			// panel's own token carries no scope claim.
			name:          "a scopeless token passes while scopes are not required",
			requireScopes: false,
			subject:       &Subject{ID: "operator"},
			want:          http.StatusOK,
			reached:       true,
		},
		{
			name:          "a scopeless token is refused once scopes are required",
			requireScopes: true,
			subject:       &Subject{ID: "operator"},
			want:          http.StatusForbidden,
		},
		{
			name:          "a token holding the scope passes",
			requireScopes: true,
			subject:       &Subject{ID: "ci", Scopes: []string{ScopeDeploy}},
			want:          http.StatusOK,
			reached:       true,
		},
		{
			name:          "a token holding several scopes including this one passes",
			requireScopes: false,
			subject:       &Subject{ID: "ci", Scopes: []string{ScopeRead, ScopeDeploy}},
			want:          http.StatusOK,
			reached:       true,
		},
		{
			// A scoped token is held to its scopes whether or not they are
			// required, which is what makes this real authorization for the one
			// caller that has them.
			name:          "a token holding a different scope is refused",
			requireScopes: false,
			subject:       &Subject{ID: "ci", Scopes: []string{ScopeRead}},
			want:          http.StatusForbidden,
		},
		{
			name:          "a request carrying no verified caller is unauthorized",
			requireScopes: false,
			subject:       nil,
			want:          http.StatusUnauthorized,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reached := false
			handler := NewGuard(test.requireScopes).Require(ScopeDeploy,
				func(w http.ResponseWriter, _ *http.Request) {
					reached = true
					w.WriteHeader(http.StatusOK)
				})

			ctx := context.Background()
			if test.subject != nil {
				ctx = WithSubject(ctx, *test.subject)
			}
			request := httptest.NewRequestWithContext(ctx, http.MethodPut, "/api/helm/x", nil)

			recorder := httptest.NewRecorder()
			handler(recorder, request)

			if recorder.Code != test.want {
				t.Errorf("status = %d, want %d", recorder.Code, test.want)
			}
			if reached != test.reached {
				t.Errorf("handler reached = %v, want %v", reached, test.reached)
			}
		})
	}
}

// A machine client cannot ask a person what went wrong, so a refusal has to say
// which scope it wanted. RFC 6750 is how it says it.
func TestGuardRefusalNamesTheScopeItWanted(t *testing.T) {
	handler := NewGuard(true).Require(ScopeDeploy, func(http.ResponseWriter, *http.Request) {
		t.Fatal("the handler ran for a caller that should have been refused")
	})

	request := httptest.NewRequestWithContext(
		WithSubject(context.Background(), Subject{ID: "ci"}), http.MethodPut, "/api/helm/x", nil)
	recorder := httptest.NewRecorder()
	handler(recorder, request)

	challenge := recorder.Header().Get("WWW-Authenticate")
	for _, want := range []string{`error="insufficient_scope"`, `scope="admin:deploy"`} {
		if !strings.Contains(challenge, want) {
			t.Errorf("WWW-Authenticate = %q, want it to contain %q", challenge, want)
		}
	}
}
