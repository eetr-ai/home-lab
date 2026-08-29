package helm

import "errors"

// The conditions this slice reports.
var (
	// ErrNotConfigured reports that this lab has no chart catalog and no managed
	// namespaces, so there is nothing for this slice to manage. Distinct from
	// ErrNotFound: the capability exists and was not switched on, rather than the
	// release being absent.
	ErrNotConfigured = errors.New("helm is not configured")
	// ErrInvalidName reports a release name, namespace, or version this slice
	// will not accept.
	ErrInvalidName = errors.New("invalid name")
	// ErrNotFound reports a release Helm's storage does not have.
	ErrNotFound = errors.New("not found")
	// ErrForbidden reports something the panel's ServiceAccount may not do.
	ErrForbidden = errors.New("forbidden")
	// ErrProtected reports a namespace Helm may never write to.
	ErrProtected = errors.New("protected namespace")
	// ErrUnmanaged reports a namespace this lab has not made a Helm target.
	ErrUnmanaged = errors.New("namespace is not helm-managed")
)
