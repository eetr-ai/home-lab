package mongo

import "errors"

// The conditions this slice reports. Handlers map them to status codes; nothing
// below the handler knows about HTTP.
var (
	// ErrInvalidName reports a database, collection, or user name this slice will
	// not send to the server.
	ErrInvalidName = errors.New("invalid name")
	// ErrNotFound reports something the server does not have.
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists reports a name already taken.
	ErrAlreadyExists = errors.New("already exists")
	// ErrProtected reports something this panel refuses to touch, regardless of
	// what the account it connects as is permitted to do.
	ErrProtected = errors.New("protected")
	// ErrWeakPassword reports a user password below the minimum length.
	ErrWeakPassword = errors.New("weak password")
	// ErrInvalidRole reports a role this panel will not grant.
	ErrInvalidRole = errors.New("invalid role")
	// ErrQueryFailed reports a query the server refused. It carries MongoDB's own
	// message, which names the offending operator — the useful part.
	ErrQueryFailed = errors.New("query failed")
	// ErrInvalidQuery reports a query this panel will not send — one carrying an
	// operator that executes code on the server.
	ErrInvalidQuery = errors.New("invalid query")
)
