package auth

import (
	"fmt"
	"net/http"

	httpx "github.com/eetr-ai/home-lab/admin/api/internal/http"
)

// Guard refuses a caller whose token does not carry the scope a route needs.
//
// A guard rather than a plain function because the answer depends on
// configuration — whether a token with no scopes at all is acceptable — and a
// package-level switch would be a setting no test could vary and no reader could
// find.
//
// It is applied per route rather than as a second middleware, because the scope a
// route needs is known only by the slice that serves it. Wrap the handler inside
// the registration rather than wrapping the mux: internal/openapi's coverage test
// finds routes by reading the method-and-path string literal out of the source,
// and it can only see one that is still an argument to HandleFunc.
type Guard struct {
	requireScopes bool
}

// NewGuard builds the guard.
//
// requireScopes decides what a token naming no scopes may do. While it is false
// such a token is unrestricted, and this is not authorization for the operator's
// token — it is a constraint on clients that declare themselves. It is real
// authorization for a token that does carry scopes, which is the only kind this
// lab issues to a pipeline, and that is the credential worth bounding.
//
// Turning it on is a deliberate, separate step, and it cannot be taken until the
// identity provider issues the panel's own client a token naming admin:read and
// admin:write. Until then it would lock the panel out of its own API.
func NewGuard(requireScopes bool) *Guard {
	return &Guard{requireScopes: requireScopes}
}

// Require wraps a handler so only a caller permitted by the scope reaches it.
func (g *Guard) Require(scope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		subject, ok := SubjectFrom(r.Context())
		if !ok {
			// Unreachable behind Middleware, and answered rather than asserted: a
			// route wired outside the authenticated mux should surface as a clear
			// refusal rather than as a panic or, far worse, as an allow.
			httpx.Error(w, http.StatusUnauthorized, "unauthorized",
				"the request carries no verified caller")
			return
		}

		// A token naming no scopes is unrestricted unless configuration says
		// otherwise; a token naming some is held to them. HasScope carries the
		// first half of that rule, requireScopes the second.
		if g.requireScopes && len(subject.Scopes) == 0 {
			insufficientScope(w, scope)
			return
		}
		if !subject.HasScope(scope) {
			insufficientScope(w, scope)
			return
		}

		next(w, r)
	}
}

// insufficientScope refuses a caller that authenticated but may not do this.
//
// 403 rather than 401: the credential was good, so there is nothing to retry with
// a fresh token. The challenge header is what makes the refusal actionable — a
// pipeline cannot ask anyone what went wrong, and "forbidden" alone does not say
// that a scope is missing or which one.
func insufficientScope(w http.ResponseWriter, scope string) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(
		`Bearer realm="home-lab-admin", error="insufficient_scope", scope=%q`, scope))
	httpx.Error(w, http.StatusForbidden, "forbidden",
		"the bearer token does not carry the "+scope+" scope")
}
