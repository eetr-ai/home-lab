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
			scope: "admin:read admin:deploy",
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
			scope: "admin:deploy",
			scp:   `["admin:read"]`,
			want:  []string{"admin:deploy"},
		},
		{
			name: "no claim at all",
			want: nil,
		},
		{
			name:  "a claim that is only whitespace",
			scope: "   ",
			want:  nil,
		},
		{
			name:  "ragged whitespace between scopes",
			scope: "  admin:read \t admin:write  ",
			want:  []string{"admin:read", "admin:write"},
		},
		{
			name: "scp is a number, which is nobody's scope",
			scp:  `42`,
			want: nil,
		},
		{
			name: "scp is an array with a non-string in it",
			scp:  `["admin:read", 7]`,
			want: nil,
		},
		{
			name:  "a duplicated scope is carried once",
			scope: "admin:read admin:read",
			want:  []string{"admin:read"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var scp json.RawMessage
			if test.scp != "" {
				scp = json.RawMessage(test.scp)
			}

			got := parseScopes(test.scope, scp)
			if !slices.Equal(got, test.want) {
				t.Errorf("parseScopes(%q, %s) = %v, want %v", test.scope, test.scp, got, test.want)
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.subject.HasScope(test.want); got != test.ok {
				t.Errorf("HasScope(%q) = %v, want %v", test.want, got, test.ok)
			}
		})
	}
}
