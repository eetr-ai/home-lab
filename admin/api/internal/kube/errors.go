package kube

import "errors"

// The conditions this slice reports.
var (
	// ErrInvalidName reports a namespace name that is not a valid Kubernetes one.
	ErrInvalidName = errors.New("invalid name")
	// ErrNotFound reports something the cluster does not have.
	ErrNotFound = errors.New("not found")
	// ErrForbidden reports something the panel's ServiceAccount may not read.
	ErrForbidden = errors.New("forbidden")
	// ErrUnsupportedKind reports an operation asked of a kind that has no such
	// thing — scaling a DaemonSet, whose replica count comes from how many nodes
	// it matches rather than from anything that can be set.
	ErrUnsupportedKind = errors.New("unsupported kind")
	// ErrConflict reports a write refused because the object changed underneath
	// it — two operators scaling the same workload at once.
	ErrConflict = errors.New("conflict")
)
