package helm

import "errors"

// The conditions this slice reports.
var (
	// ErrNotConfigured reports that no namespace is enrolled as a Helm target, so
	// there is nothing for this slice to manage. Distinct from ErrNotFound: the
	// capability exists and was not switched on, rather than the release being
	// absent.
	ErrNotConfigured = errors.New("helm is not configured")
	// ErrNoDeploymentStore reports that this lab configured no database for
	// declared deployments, so there is nowhere to keep one.
	//
	// Separate from ErrNotConfigured because the two were one, and the message an
	// operator got for this was "no namespaces are configured for Helm" -- which
	// is a different problem with a different fix, and sends somebody to enrol a
	// namespace that is already enrolled. Reading the releases still works
	// without a store; only declaring does.
	ErrNoDeploymentStore = errors.New("no deployment store is configured")
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
	// ErrStoreUnavailable reports that the record of what this lab has declared
	// could not be reached. Deliberately distinct from every "the record says no"
	// error: only this one is a 503, and only this one means retrying might work.
	ErrStoreUnavailable = errors.New("deployment store unavailable")
	// ErrRepositoryUnreachable reports a chart repository that could not be read.
	// Not the caller's fault, and deliberately not a 400.
	ErrRepositoryUnreachable = errors.New("chart repository unreachable")
	// ErrNoPodYet reports a job whose pod has not been scheduled.
	//
	// Deliberately distinct from ErrNotFound, because the two mean opposite
	// things to a caller following an operation: this one says try again in a
	// moment, and not-found says stop. Every job passes through it for the first
	// moment of its life, so a client that conflates them shows "the log is gone"
	// for every operation it watches from the start.
	ErrNoPodYet = errors.New("no pod yet")
)
