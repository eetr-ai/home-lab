// Package auth authenticates callers of the admin API.
//
// The API is an OAuth 2.1 resource server and nothing else: every caller presents
// a bearer access token issued by the configured OpenID Connect provider, and the
// token is the only credential. There are no API keys, because the two callers
// that matter — the panel's own server and, later, an assistant agent — both act
// on behalf of a person who signed in, and a second credential type would be a
// second thing that can be leaked while proving less.
package auth

import (
	"context"
	"net/http"
	"strings"
)

// Subject identifies the caller a verified token belongs to.
type Subject struct {
	// ID is the provider's stable subject claim. It is the identifier to key
	// anything per-user on, and it does not change between sign-ins.
	ID string
	// Email is present when the token carries the email claim, and empty
	// otherwise. Useful for logs; never an identifier.
	Email string
	// Scopes are the permissions the token names, and nil when it names none.
	// Nil is not "no permissions" — see HasScope in scope.go for why the
	// difference matters.
	Scopes []string
}

// TokenVerifier verifies a raw bearer token and reports whose it is.
//
// Declared here rather than beside its implementation because this is where it is
// consumed, which is what lets the middleware be tested without an identity
// provider on the other end of a network call.
type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (Subject, error)
}

type contextKey struct{}

var subjectKey contextKey

// SubjectFrom returns the verified caller carried on a request context, and
// whether there was one. A handler behind Middleware can rely on it being there.
func SubjectFrom(ctx context.Context) (Subject, bool) {
	subject, ok := ctx.Value(subjectKey).(Subject)
	return subject, ok
}

// Middleware rejects any request that does not carry a token the verifier
// accepts, and puts the caller on the context of the ones that do.
func Middleware(verifier TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawToken, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				unauthorized(w, "a bearer token is required")
				return
			}

			subject, err := verifier.Verify(r.Context(), rawToken)
			if err != nil {
				// Deliberately not echoed. Whether a token expired, was signed by
				// an unknown key, or names the wrong audience is useful to an
				// operator reading logs and useful to an attacker reading
				// responses; only one of them gets it.
				unauthorized(w, "the bearer token is not valid")
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), subjectKey, subject)))
		})
	}
}

// bearerToken pulls the credential out of an Authorization header. RFC 6750
// names the scheme case-insensitively and clients differ, so the comparison is
// too. A header carrying anything other than exactly one token is refused rather
// than guessed at.
func bearerToken(header string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(header))
	if len(fields) != 2 {
		return "", false
	}
	if !strings.EqualFold(fields[0], "Bearer") {
		return "", false
	}
	return fields[1], true
}

// unauthorized answers a request that carried no usable credential. RFC 6750
// requires the challenge header: without it a client is told no and given nothing
// to act on.
func unauthorized(w http.ResponseWriter, detail string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="home-lab-admin"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"error":"unauthorized","message":"` + detail + `"}`))
}
