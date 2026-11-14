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
	// ErrAlreadyExists reports a user name already taken.
	ErrAlreadyExists = errors.New("already exists")
	// ErrProtected reports something this panel refuses to touch whatever its
	// account could do: the default user.
	ErrProtected = errors.New("protected")
	// ErrWeakPassword reports a password below the minimum length.
	ErrWeakPassword = errors.New("weak password")
	// ErrNotPersistent reports a server with no aclfile, where a user would not
	// survive a restart.
	ErrNotPersistent = errors.New("users are not persistent")
	// ErrNotSaved reports a user change the server would not write to its
	// aclfile. The message says whether it was undone.
	ErrNotSaved = errors.New("not saved")
	// ErrBusy reports a user change refused because another one held the ACL
	// lock for longer than this one was willing to wait.
	ErrBusy = errors.New("busy")

	// errNotUndone marks the worse half of ErrNotSaved: the change is live in
	// memory and not on disk, and will be lost at the next restart.
	errNotUndone = errors.New("not undone")
)
