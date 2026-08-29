package auth

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestParseScopes(t *testing.T) {
	tests := []struct {
		name  string
		scope string
		scp   string
		want  []string
	}{
		{
			name:  "the space-delimited scope claim",
			scope: `"admin:read admin:deploy"`,
			want:  []string{"admin:read", "admin:deploy"},
		},
		{
			// A provider is within its rights to spell `scope` as an array, and
			// one that does must not read as a token with no scopes at all --
			// a scopeless token is unrestricted, so failing to parse this fails
			// open.
			name:  "scope as an array",
			scope: `["admin:read","admin:deploy"]`,
			want:  []string{"admin:read", "admin:deploy"},
		},
		{
			name: "scp as an array",
			scp:  `["admin:read","admin:write"]`,
			want: []string{"admin:read", "admin:write"},
		},
		{
			name: "scp as a space-delimited string",
			scp:  `"admin:read admin:write"`,
			want: []string{"admin:read", "admin:write"},
		},
		{
			name:  "scope wins when both are present",
			scope: `"admin:deploy"`,
			scp:   `["admin:read"]`,
			want:  []string{"admin:deploy"},
		},
		{
			name: "no claim at all",
			want: nil,
		},
		{
			name:  "a claim that is only whitespace",
			scope: `"   "`,
			want:  nil,
		},
		{
			name:  "ragged whitespace between scopes",
			scope: `"  admin:read \t admin:write  "`,
			want:  []string{"admin:read", "admin:write"},
		},
		{
			name: "scp is a number, which is nobody's scope",
			scp:  `42`,
			want: nil,
		},
		{
			name:  "scope is a number and scp is good",
			scope: `42`,
			scp:   `["admin:read"]`,
			want:  []string{"admin:read"},
		},
		{
			// An empty scope claim is not an answer, so the other one is read.
			name:  "scope is present but empty",
			scope: `""`,
			scp:   `["admin:read"]`,
			want:  []string{"admin:read"},
		},
		{
			name: "scp is an array with a non-string in it",
			scp:  `["admin:read", 7]`,
			want: nil,
		},
		{
			name:  "a duplicated scope is carried once",
			scope: `"admin:read admin:read"`,
			want:  []string{"admin:read"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var scope, scp json.RawMessage
			if test.scope != "" {
				scope = json.RawMessage(test.scope)
			}
			if test.scp != "" {
				scp = json.RawMessage(test.scp)
			}

			got := parseScopes(scope, scp)
			if !slices.Equal(got, test.want) {
				t.Errorf("parseScopes(%s, %s) = %v, want %v", test.scope, test.scp, got, test.want)
			}
		})
	}
}

func TestSubjectHasScope(t *testing.T) {
	tests := []struct {
		name    string
		subject Subject
		want    string
		ok      bool
	}{
		{
			name:    "the scope is held",
			subject: Subject{Scopes: []string{"admin:read", "admin:deploy"}},
			want:    "admin:deploy",
			ok:      true,
		},
		{
			name:    "a different scope is held",
			subject: Subject{Scopes: []string{"admin:read"}},
			want:    "admin:deploy",
			ok:      false,
		},
		{
			// The rule the whole design rests on: scopes narrow, they never
			// widen. A token that names none is as unrestricted as it was before
			// scopes existed.
			name:    "a token carrying no scopes is not narrowed",
			subject: Subject{},
			want:    "admin:deploy",
			ok:      true,
		},
		{
			// Every token eetr-auth issues the panel's own client looks exactly
			// like this. These are authentication scopes -- how the caller signed
			// in -- and say nothing about what it may do here, so they must not
			// narrow anything. Counting them refused every real token on every
			// scoped route, which is how this was found: against the live
			// provider, not in a test.
			name:    "the OIDC scopes on every real token do not narrow",
			subject: Subject{Scopes: []string{"openid", "profile", "email"}},
			want:    "admin:deploy",
			ok:      true,
		},
		{
			// eetr-auth's coarse administrative scope. Still not one of ours, so
			// it does not narrow either -- and it cannot be read as granting
			// everything, because that would be widening.
			name:    "the provider's own admin scope does not narrow",
			subject: Subject{Scopes: []string{"openid", "email", "admin"}},
			want:    "admin:deploy",
			ok:      true,
		},
		{
			// ...but once a token names one of ours, the foreign ones are still
			// ignored and the narrowing is real.
			name:    "our scopes narrow even alongside foreign ones",
			subject: Subject{Scopes: []string{"openid", "email", "admin:read"}},
			want:    "admin:deploy",
			ok:      false,
		},
		{
			name:    "and permit what they name",
			subject: Subject{Scopes: []string{"openid", "email", "admin:deploy"}},
			want:    "admin:deploy",
			ok:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.subject.HasScope(test.want); got != test.ok {
				t.Errorf("HasScope(%q) = %v, want %v", test.want, got, test.ok)
			}
		})
	}
}

// A client_credentials token carries no subject and no email on this provider,
// so the client id is the only identity there is. Without it, everything a
// pipeline changes is attributed to nobody.
func TestSubjectCarriesTheClientID(t *testing.T) {
	cases := map[string]struct {
		clientID, azp string
		want          string
	}{
		"client_id, as RFC 9068 spells it": {clientID: "home-lab-ci", want: "home-lab-ci"},
		"azp, as OpenID Connect spells it": {azp: "home-lab-ci", want: "home-lab-ci"},
		"client_id wins when both are set": {clientID: "first", azp: "second", want: "first"},
		"neither":                          {want: ""},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			got := stringClaim(rawOrNil(testCase.clientID), rawOrNil(testCase.azp))
			if got != testCase.want {
				t.Errorf("want %q, got %q", testCase.want, got)
			}
		})
	}
}

// A claim of the wrong shape must not stop a sibling being read: that is the
// same failure that once made a scoped token look scopeless, and unrestricted.
func TestAClaimOfTheWrongShapeDoesNotHideTheNextOne(t *testing.T) {
	if got := stringClaim(json.RawMessage(`{"not":"a string"}`), rawOrNil("fallback")); got != "fallback" {
		t.Errorf("want the readable claim to win, got %q", got)
	}
	if got := stringClaim(json.RawMessage(`12345`), json.RawMessage(`[1,2]`)); got != "" {
		t.Errorf("want no claim, got %q", got)
	}
}

func rawOrNil(value string) json.RawMessage {
	if value == "" {
		return nil
	}
	encoded, _ := json.Marshal(value)
	return encoded
}
