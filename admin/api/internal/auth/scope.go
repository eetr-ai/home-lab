package auth

import (
	"encoding/json"
	"slices"
	"strings"
)

// The scopes this API recognises.
//
// Three, deliberately coarse. A scope per endpoint would be a permission model
// nobody maintains and every client gets wrong; these separate the three things
// that differ in consequence — reading the lab, changing what is already running,
// and putting something new on the cluster.
const (
	// ScopeRead permits every GET.
	ScopeRead = "admin:read"
	// ScopeWrite permits changing something that already exists: restarting a
	// workload, scaling it, creating or deleting a namespace.
	ScopeWrite = "admin:write"
	// ScopeDeploy permits installing, upgrading, rolling back, and uninstalling a
	// Helm release. Separate from ScopeWrite because this is the one a pipeline
	// holds, and a pipeline should not also be able to scale a workload to zero.
	ScopeDeploy = "admin:deploy"
)

// HasScope reports whether this caller may do something the given scope guards.
//
// Scopes narrow; they never widen. A token that carries no scopes at all is
// unrestricted, which is what every token this API has ever accepted looked like
// and is why introducing scopes breaks nothing. A token that carries scopes is
// held to exactly them.
//
// Whether a scopeless token is acceptable in the first place is a separate
// question, and not this one's to answer.
func (s Subject) HasScope(scope string) bool {
	if len(s.Scopes) == 0 {
		return true
	}
	return slices.Contains(s.Scopes, scope)
}

// parseScopes reads the scopes out of the two claims providers actually use.
//
// `scope` is the OAuth spelling and RFC 9068's; `scp` is what several providers
// emit instead. Neither has one shape in practice — both appear as a
// space-delimited string and as an array of strings, depending on who issued the
// token — so both claims are read both ways. When `scope` yields anything it
// wins, because it is the one the specification names.
//
// All four combinations are handled because getting this wrong is invisible and
// it fails open: a token whose scopes were not found looks exactly like a token
// issued without any, and a token with no scopes is unrestricted. The difference
// has to be found here or it is never found at all.
//
// Anything else — a number, an array with a number in it — yields no scopes
// rather than a panic or a partial read.
func parseScopes(scope, scp json.RawMessage) []string {
	if scopes := readScopeClaim(scope); len(scopes) > 0 {
		return scopes
	}
	return readScopeClaim(scp)
}

// readScopeClaim reads one claim, whichever of its two shapes it arrived in.
func readScopeClaim(claim json.RawMessage) []string {
	if len(claim) == 0 {
		return nil
	}

	var list []string
	if err := json.Unmarshal(claim, &list); err == nil {
		return dedupe(list)
	}

	var delimited string
	if err := json.Unmarshal(claim, &delimited); err == nil {
		return dedupe(strings.Fields(delimited))
	}

	return nil
}

// dedupe keeps the first occurrence of each scope and drops empties, so a claim
// spelled with a repeated entry compares the same as one without.
func dedupe(scopes []string) []string {
	var unique []string
	for _, scope := range scopes {
		if scope == "" || slices.Contains(unique, scope) {
			continue
		}
		unique = append(unique, scope)
	}
	return unique
}
