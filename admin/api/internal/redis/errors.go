package redis

import "errors"

// The conditions this slice reports. Handlers map them to status codes; nothing
// below the handler knows about HTTP.
var (
	// ErrInvalidRequest reports a request this slice will not send to the server:
	// a cursor that is not one, a pattern carrying a control character, an empty
	// key name.
	ErrInvalidRequest = errors.New("invalid request")
	// ErrNotFound reports a key or user the server does not have.
	ErrNotFound = errors.New("not found")
)
