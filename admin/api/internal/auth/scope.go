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
// `scope` is the OAuth spelling and RFC 9068's: one space-delimited string. `scp`
// is what several providers emit instead, and they disagree about whether it is
// an array or a string — so both shapes are read. When both claims are present
// `scope` wins, because it is the one the specification names.
//
// Both are handled because getting this wrong is invisible: a token whose scopes
// were not found looks exactly like a token that was issued without any, and the
// difference only shows up as a refusal nobody can explain.
//
// Anything else — a number, an array with a number in it — yields no scopes
// rather than a panic or a partial read.
func parseScopes(scope string, scp json.RawMessage) []string {
	if fields := strings.Fields(scope); len(fields) > 0 {
		return dedupe(fields)
	}

	if len(scp) == 0 {
		return nil
	}

	var list []string
	if err := json.Unmarshal(scp, &list); err == nil {
		return dedupe(list)
	}

	var delimited string
	if err := json.Unmarshal(scp, &delimited); err == nil {
		if fields := strings.Fields(delimited); len(fields) > 0 {
			return dedupe(fields)
		}
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
