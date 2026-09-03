package postgres

import (
	"fmt"
	"regexp"
	"strings"
)

// maxIdentifierLength is PostgreSQL's NAMEDATALEN - 1. A longer name is not
// rejected by the server but silently truncated, which would leave the panel
// reporting a name that does not exist.
const maxIdentifierLength = 63

// safeIdentifier is what this slice is willing to put into a statement.
//
// Deliberately narrower than what PostgreSQL accepts: a quoted identifier may
// contain almost anything, but nobody administering this lab needs a database
// called `my db"; DROP`, and an allowlist is far easier to be sure of than an
// escaper.
var safeIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$]*$`)

// safeExtensionName is the same, plus the hyphen.
//
// Extension names are not chosen by the operator — they are whatever the
// extension is called — and several of PostgreSQL's own contain a hyphen, most
// obviously uuid-ossp. Refusing them would make those extensions uninstallable
// through this panel for no gain: the name is quoted either way, so a hyphen is
// as inert inside the quotes as an underscore.
var safeExtensionName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_$-]*$`)

// quoteExtensionName validates an extension name and returns it quoted.
func quoteExtensionName(name string) (string, error) {
	if len(name) > maxIdentifierLength {
		return "", fmt.Errorf("%w: names may be at most %d characters", ErrInvalidName, maxIdentifierLength)
	}
	if !safeExtensionName.MatchString(name) {
		return "", fmt.Errorf(
			"%w: extension names must start with a letter or underscore and contain only "+
				"letters, digits, underscores, hyphens, and $",
			ErrInvalidName)
	}
	return `"` + name + `"`, nil
}

// quoteName quotes an identifier read from the catalog — a schema, table, or
// column the panel is browsing — for interpolation into a statement.
//
// Unlike quoteIdentifier's allowlist, it accepts any name PostgreSQL itself
// allows, because these names are not invented by a caller: they are the names of
// objects the tree already listed, so refusing a capitalised, spaced, or quoted
// one would simply make a real table un-browsable. Injection is handled the way
// PostgreSQL's own quote_ident does — double every embedded quote and wrap — which
// is sound regardless of standard_conforming_strings, since that setting governs
// string literals, not identifiers. A null byte cannot appear in an identifier and
// is refused rather than escaped.
func quoteName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%w: an identifier may not be empty", ErrInvalidName)
	}
	if len(name) > maxIdentifierLength {
		return "", fmt.Errorf("%w: names may be at most %d characters", ErrInvalidName, maxIdentifierLength)
	}
	if strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("%w: an identifier may not contain a null byte", ErrInvalidName)
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`, nil
}

// quoteIdentifier validates a database or role name and returns it
// quoted, ready to interpolate into a statement.
//
// PostgreSQL cannot parameterize an identifier — `CREATE DATABASE $1` is not
// valid SQL — so every DDL statement in this slice interpolates its name, and
// this function is the whole of the injection defense for all of them. Nothing
// here builds a statement from a name that did not come through here.
func quoteIdentifier(name string) (string, error) {
	if len(name) > maxIdentifierLength {
		return "", fmt.Errorf("%w: names may be at most %d characters", ErrInvalidName, maxIdentifierLength)
	}
	if !safeIdentifier.MatchString(name) {
		return "", fmt.Errorf(
			"%w: names must start with a letter or underscore and contain only letters, digits, underscores, and $",
			ErrInvalidName)
	}
	return `"` + name + `"`, nil
}
