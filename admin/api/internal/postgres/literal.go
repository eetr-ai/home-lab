package postgres

import (
	"fmt"
	"strings"
)

// quoteLiteral returns a string as a PostgreSQL literal.
//
// It exists for the same reason quoteIdentifier does: `CREATE ROLE x PASSWORD $1`
// is not valid SQL, because utility statements do not take bind parameters. A
// password has to be interpolated, so it has to be quoted correctly.
//
// Doubling the single quotes is sufficient **only** while
// standard_conforming_strings is on, which has been the default since PostgreSQL
// 9.1 and which repo.go verifies on connect. With it off, a backslash would begin
// an escape sequence and `\'` would slip a quote through the doubling.
func quoteLiteral(value string) (string, error) {
	// A NUL cannot appear in a PostgreSQL text value at all; the protocol would
	// truncate the string at it, silently setting a shorter password than asked.
	if strings.ContainsRune(value, 0) {
		return "", fmt.Errorf("%w: values may not contain a null byte", ErrInvalidName)
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'", nil
}
