package postgres

import (
	"strings"
	"testing"
)

func TestQuoteLiteral(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "an ordinary password", input: "correct horse battery", want: `'correct horse battery'`},
		{name: "a single quote is doubled", input: "it's", want: `'it''s'`},
		// The classic escape: closing the literal and appending a statement.
		{name: "an attempted break-out", input: `x'; DROP ROLE admin; --`, want: `'x''; DROP ROLE admin; --'`},
		// Literal with standard_conforming_strings on, which repo.go verifies.
		{name: "a backslash stays a backslash", input: `a\b`, want: `'a\b'`},
		{name: "a backslash before a quote", input: `a\'b`, want: `'a\''b'`},
		{name: "empty", input: "", want: `''`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := quoteLiteral(test.input)
			if err != nil {
				t.Fatalf("quoteLiteral(%q) = %v", test.input, err)
			}
			if got != test.want {
				t.Errorf("quoteLiteral(%q) = %s, want %s", test.input, got, test.want)
			}
			// Whatever it produced must be one balanced literal: every quote
			// inside the body doubled, so the count is even.
			body := strings.TrimSuffix(strings.TrimPrefix(got, "'"), "'")
			if strings.Count(body, "'")%2 != 0 {
				t.Errorf("quoteLiteral(%q) = %s, which does not close cleanly", test.input, got)
			}
		})
	}
}

// A NUL would be truncated by the wire protocol, silently setting a shorter
// password than the caller asked for — which they would then not be able to use.
func TestQuoteLiteralRejectsNullBytes(t *testing.T) {
	if _, err := quoteLiteral("abc\x00def"); err == nil {
		t.Fatal("quoteLiteral() accepted a null byte")
	}
}
