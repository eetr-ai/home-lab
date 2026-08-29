package helm

import "errors"

// The conditions this slice reports.
var (
	// ErrNotConfigured reports that this lab has made no namespace a Helm target,
	// so there is nothing for this slice to manage. Distinct from ErrNotFound: the
	// capability exists and was not switched on, rather than the release being
	// absent.
	ErrNotConfigured = errors.New("helm is not configured")
	// ErrInvalidName reports a release name, namespace, or version this slice
	// will not accept.
	ErrInvalidName = errors.New("invalid name")
	// ErrInvalidChartRef reports a chart reference this API will not fetch.
	ErrInvalidChartRef = errors.New("invalid chart reference")
	// ErrInvalidValues reports a values document that is not YAML, or is YAML
	// that is not a mapping. The wrapped message names the line.
	ErrInvalidValues = errors.New("invalid values")
	// ErrValuesTooLarge reports values too big to be stored in a Secret.
	ErrValuesTooLarge = errors.New("values too large")
	// ErrNotFound reports a release Helm's storage does not have, or a deployment
	// this lab has no record of.
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists reports a release name already in use in the namespace.
	ErrAlreadyExists = errors.New("already exists")
	// ErrInProgress reports an operation already running against the release.
	ErrInProgress = errors.New("operation in progress")
	// ErrForbidden reports something the panel's ServiceAccount may not do.
	ErrForbidden = errors.New("forbidden")
	// ErrProtected reports a namespace Helm may never write to.
	ErrProtected = errors.New("protected namespace")
	// ErrUnmanaged reports a namespace this lab has not made a Helm target.
	ErrUnmanaged = errors.New("namespace is not helm-managed")
	// ErrRepositoryUnreachable reports a chart repository that could not be read.
	// Not the caller's fault, and deliberately not a 400.
	ErrRepositoryUnreachable = errors.New("chart repository unreachable")
)
