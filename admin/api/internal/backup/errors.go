package backup

import "errors"

// The conditions this slice reports.
var (
	// ErrNotConfigured reports that this lab has no Google OAuth client, so there
	// is no account to connect and nowhere to upload to. Distinct from
	// ErrNotConnected: this one is fixed in the chart, that one in the panel.
	ErrNotConfigured = errors.New("google drive is not configured")
	// ErrNotConnected reports an OAuth client with no account behind it yet.
	ErrNotConnected = errors.New("no google account is connected")
	// ErrInvalid reports a request this slice will not accept. The wrapped
	// message says which field and why.
	ErrInvalid = errors.New("invalid request")
	// ErrGoogleRefused reports Google answering no: a code that was already used
	// or has expired, a revoked grant, a client secret that does not match.
	// The caller can do something about it, so it is not a 502.
	ErrGoogleRefused = errors.New("google refused the request")
	// ErrGoogleUnreachable reports a call to Google that never got an answer.
	ErrGoogleUnreachable = errors.New("google is unreachable")
	// ErrNotFound reports a schedule this lab does not have.
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists reports a schedule name already in use.
	ErrAlreadyExists = errors.New("already exists")
	// ErrInProgress reports a run of the schedule already going. Two runs at once
	// would both prune, and each would count the other's upload and keep one
	// fewer than asked.
	ErrInProgress = errors.New("a run is already in progress")
	// ErrForbidden reports something the panel's ServiceAccount may not do. It is
	// a missing grant in the chart, never the caller's fault.
	ErrForbidden = errors.New("forbidden")
)
