package postgres

import (
	"strings"
	"testing"
)

// PostgreSQL cannot parameterize an identifier: `CREATE DATABASE $1` is not valid
// SQL, so a database or role name has to be interpolated into the statement. That
// makes this function the whole of the injection defense for every DDL statement
// in this slice, which is why it is an allowlist rather than an escaper.
//
// Names Postgres would accept in quotes — with spaces, punctuation, or embedded
// quotes — are refused. They are legal and nobody administering this lab needs
// one, and "reject what we do not recognize" is far easier to be sure of than
// "escape everything correctly".
func TestQuoteIdentifier(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "a simple name", input: "app", want: `"app"`},
		{name: "underscores", input: "app_data", want: `"app_data"`},
		{name: "a leading underscore", input: "_internal", want: `"_internal"`},
		{name: "digits after the first character", input: "app2", want: `"app2"`},
		{name: "a dollar sign", input: "app$data", want: `"app$data"`},
		{name: "mixed case is preserved by the quoting", input: "AppData", want: `"AppData"`},
		{name: "the maximum length", input: strings.Repeat("a", 63), want: `"` + strings.Repeat("a", 63) + `"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := quoteIdentifier(test.input)
			if err != nil {
				t.Fatalf("quoteIdentifier(%q) = %v, want %q", test.input, err, test.want)
			}
			if got != test.want {
				t.Errorf("quoteIdentifier(%q) = %s, want %s", test.input, got, test.want)
			}
		})
	}
}

func TestQuoteIdentifierRejects(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "only whitespace", input: "   "},
		{name: "a leading digit", input: "1app"},
		{name: "a space", input: "app data"},
		{name: "a hyphen", input: "app-data"},
		{name: "a dot", input: "app.data"},

		// Each of these is an injection attempt that a naive `"` + name + `"`
		// would let through, or that a bare interpolation would let through
		// outright.
		{name: "an embedded double quote", input: `app"`},
		{name: "a quote closing and reopening", input: `a" ; DROP DATABASE "b`},
		{name: "a statement separator", input: "app; DROP DATABASE postgres"},
		{name: "a line comment", input: "app--"},
		{name: "a block comment", input: "app/*x*/"},
		{name: "a single quote", input: "app'"},
		{name: "a backslash", input: `app\`},
		{name: "a newline", input: "app\nDROP DATABASE postgres"},
		{name: "a null byte", input: "app\x00"},

		{name: "longer than postgres allows", input: strings.Repeat("a", 64)},
		// Multi-byte characters are legal Postgres identifiers but the length
		// limit is in bytes, and allowing them buys nothing here.
		{name: "not ascii", input: "café"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := quoteIdentifier(test.input)
			if err == nil {
				t.Fatalf("quoteIdentifier(%q) = %s, want an error", test.input, got)
			}
		})
	}
}

// Extension names are whatever the extension is called, and several of
// PostgreSQL's own contain a hyphen. Rejecting them would make those extensions
// uninstallable through the panel.
func TestQuoteExtensionName(t *testing.T) {
	accepted := []string{"vector", "pgcrypto", "pg_trgm", "uuid-ossp", "postgis_topology"}
	for _, name := range accepted {
		t.Run(name, func(t *testing.T) {
			got, err := quoteExtensionName(name)
			if err != nil {
				t.Fatalf("quoteExtensionName(%q) = %v, want it accepted", name, err)
			}
			if got != `"`+name+`"` {
				t.Errorf("quoteExtensionName(%q) = %s", name, got)
			}
		})
	}

	// The extra character is a hyphen and nothing else: everything that would
	// break out of the quotes is still refused.
	refused := []string{"", "1vector", `v"`, "v; DROP", "v--x;", "v/*x*/", "v b", "v'", "v\nDROP"}
	for _, name := range refused {
		t.Run("refused:"+name, func(t *testing.T) {
			if got, err := quoteExtensionName(name); err == nil {
				t.Fatalf("quoteExtensionName(%q) = %s, want an error", name, got)
			}
		})
	}
}

// A hyphen is still not acceptable in a database or role name, where the operator
// chooses the name and a narrower rule costs nothing.
func TestQuoteIdentifierStillRejectsHyphens(t *testing.T) {
	if _, err := quoteIdentifier("uuid-ossp"); err == nil {
		t.Fatal("quoteIdentifier accepted a hyphen")
	}
}
