package postgres

import "errors"

// The conditions this slice reports. Handlers map them to status codes; nothing
// below the handler knows about HTTP.
var (
	// ErrInvalidName reports a database, role, or extension name this slice will
	// not put into a statement.
	ErrInvalidName = errors.New("invalid name")
	// ErrNotFound reports an object the server does not have.
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists reports a name already taken.
	ErrAlreadyExists = errors.New("already exists")
	// ErrProtected reports an object this panel refuses to touch, regardless of
	// what the superuser it connects as is permitted to do.
	ErrProtected = errors.New("protected")
	// ErrWeakPassword reports a role password below the minimum length.
	ErrWeakPassword = errors.New("weak password")
	// ErrQueryFailed reports a query the server refused. It carries the server's
	// own message, which is the useful part — a syntax error names the position,
	// and a read-only refusal names the statement kind.
	ErrQueryFailed = errors.New("query failed")
)
