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

// The scope set docs/deploying-from-a-pipeline.md tells an operator to issue a
// pipeline, checked against the routes that pipeline actually calls.
//
// This is a documentation test, and it earns its place: the guide originally said
// to issue admin:deploy and nothing else, on the reasoning that reading a release
// was permitted to any accepted token. That is true only of a token naming no
// scopes at all. A pipeline's token names some, so it is held to them — and a
// deploy-only token got its 202 and then 403 on every poll, which a loop waiting
// for a terminal status reads as "not finished yet" and waits out forever.
//
// A deployer that cannot observe its own deploy is not a smaller permission. If
// this test fails, the guide is wrong, not the test.
func TestTheDocumentedPipelineScopesReachEveryRouteAPipelineCalls(t *testing.T) {
	pipeline := Subject{ID: "home-lab-ci", Scopes: []string{ScopeRead, ScopeDeploy}}

	tests := []struct {
		name  string
		scope string
	}{
		{name: "PUT the release", scope: ScopeDeploy},
		{name: "poll the release", scope: ScopeRead},
		{name: "read its history", scope: ScopeRead},
	}

	// requireScopes on, because that is where this is heading and the answer must
	// not depend on the transition.
	guard := NewGuard(true)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reached := false
			handler := guard.Require(test.scope, func(w http.ResponseWriter, _ *http.Request) {
				reached = true
				w.WriteHeader(http.StatusOK)
			})

			recorder := httptest.NewRecorder()
			handler(recorder, httptest.NewRequestWithContext(
				WithSubject(t.Context(), pipeline), http.MethodGet, "/api/helm/x", nil))

			if !reached || recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, reached = %v; the documented pipeline scopes do not "+
					"reach a route it has to call", recorder.Code, reached)
			}
		})
	}

	// ...and they stop where the guide says they stop.
	recorder := httptest.NewRecorder()
	guard.Require(ScopeWrite, func(http.ResponseWriter, *http.Request) {
		t.Fatal("the pipeline reached a route needing admin:write")
	})(recorder, httptest.NewRequestWithContext(
		WithSubject(t.Context(), pipeline), http.MethodPost, "/api/kubernetes/x", nil))

	if recorder.Code != http.StatusForbidden {
		t.Errorf("a write route answered %d, want 403", recorder.Code)
	}
}
