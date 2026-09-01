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
	// ErrProtected reports a namespace this panel may not delete. It is an
	// authorization statement about the object rather than a temporary condition,
	// so it is never worth retrying.
	ErrProtected = errors.New("protected namespace")
	// ErrAlreadyExists reports something the cluster already has.
	ErrAlreadyExists = errors.New("already exists")
	// ErrNotConfigured reports a capability this panel was not given. Helm
	// enrolment is the one: a lab that does not deploy from the panel has no
	// ClusterRoles to bind, and answering 501 says "built, not switched on" rather
	// than pretending the request was malformed.
	ErrNotConfigured = errors.New("not configured")
	// ErrNotEmpty reports a namespace still running something, refused because
	// deleting one cascades to everything in it.
	ErrNotEmpty = errors.New("not empty")
	// ErrNotManaged reports a namespace the panel holds no grant in. Separate from
	// ErrForbidden because the answer is different: the panel's ServiceAccount is
	// working exactly as configured, and what has to change is which namespaces
	// the panel manages — not its role binding.
	ErrNotManaged = errors.New("not a managed namespace")
	// ErrReserved reports a Secret this panel will not delete or rotate whatever
	// the namespace policy says, because of what the Secret is rather than where
	// it lives. Helm's release storage and a ServiceAccount's token are both
	// objects something else owns and rebuilds from; changing one from here
	// destroys history or breaks an identity, and neither is what an operator
	// managing a credential meant to do.
	//
	// This is the containment for the `delete` grant, and it is in Go rather than
	// in RBAC because RBAC has no way to express "not this type" — the same
	// reason restartPatch, not the role, is what confines the `patch` grant.
	ErrReserved = errors.New("reserved secret")
)
