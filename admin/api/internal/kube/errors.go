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
)
