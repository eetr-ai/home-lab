package mongo

import (
	"fmt"
	"regexp"
	"strings"
)

// MongoDB's own limits. A database name longer than this is refused by the
// server; a namespace — database, dot, collection — longer than 255 bytes is too.
const (
	maxDatabaseNameLength   = 63
	maxCollectionNameLength = 235
	maxUserNameLength       = 128
)

// The names this slice is willing to send.
//
// Unlike the PostgreSQL slice, this is not an injection defense: the driver sends
// BSON commands with typed fields, so a name is never parsed as syntax and cannot
// escape into one. These rules are about the names MongoDB itself cannot store or
// interprets specially — a dollar sign reads as an operator in several contexts,
// a dot separates a namespace, and the system. prefix is reserved for internal
// collections. Rejecting the rest is a narrowing this lab does not miss.
var (
	safeDatabaseName   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
	safeCollectionName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)
	safeUserName       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.@-]*$`)
)

// systemPrefix is reserved by MongoDB for its own collections.
const systemPrefix = "system."

// validateDatabaseName rejects a database name MongoDB cannot store.
func validateDatabaseName(name string) error {
	if len(name) > maxDatabaseNameLength {
		return fmt.Errorf("%w: database names may be at most %d characters",
			ErrInvalidName, maxDatabaseNameLength)
	}
	if !safeDatabaseName.MatchString(name) {
		return fmt.Errorf(
			"%w: database names must start with a letter or underscore and contain only "+
				"letters, digits, underscores, and hyphens",
			ErrInvalidName)
	}
	return nil
}

// validateCollectionName rejects a collection name MongoDB reserves or cannot
// store. The system. prefix is checked case-insensitively, because a collection
// called System.Users is confusing whether or not the server refuses it.
func validateCollectionName(name string) error {
	if len(name) > maxCollectionNameLength {
		return fmt.Errorf("%w: collection names may be at most %d characters",
			ErrInvalidName, maxCollectionNameLength)
	}
	if !safeCollectionName.MatchString(name) {
		return fmt.Errorf(
			"%w: collection names must start with a letter or underscore and contain only "+
				"letters, digits, underscores, dots, and hyphens",
			ErrInvalidName)
	}
	if strings.HasPrefix(strings.ToLower(name), systemPrefix) {
		return fmt.Errorf("%w: the %q prefix is reserved by MongoDB", ErrInvalidName, systemPrefix)
	}
	return nil
}

// validateUserName rejects a user name this slice will not create.
func validateUserName(name string) error {
	if len(name) > maxUserNameLength {
		return fmt.Errorf("%w: user names may be at most %d characters",
			ErrInvalidName, maxUserNameLength)
	}
	if !safeUserName.MatchString(name) {
		return fmt.Errorf(
			"%w: user names must start with a letter or underscore and contain only "+
				"letters, digits, underscores, dots, at signs, and hyphens",
			ErrInvalidName)
	}
	return nil
}
