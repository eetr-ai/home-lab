package kube

import (
	"fmt"
	"regexp"
)

// maxNamespaceLength is Kubernetes' own limit for a DNS-1123 label.
const maxNamespaceLength = 63

// dns1123Label is what Kubernetes accepts as a namespace name.
//
// Validating here rather than letting the API server refuse is not about safety —
// the client sends the name as a path segment through a typed client, not as
// syntax. It is about the error: the API server's rejection of a malformed name
// arrives as a 400 with a message about label formats, while this one can say
// which parameter was wrong.
var dns1123Label = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// maxSubdomainLength is Kubernetes' limit for a DNS-1123 subdomain, which is what
// a pod or a workload name is.
const maxSubdomainLength = 253

// dns1123Subdomain is what Kubernetes accepts as a pod or workload name: one or
// more labels joined by dots.
var dns1123Subdomain = regexp.MustCompile(
	`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

// The bounds on a log request.
const (
	// defaultLogTail is how much history a caller gets without asking. Enough to
	// see why something crashed, short enough to arrive at once.
	defaultLogTail = 200
	// maxLogTail caps what a caller may ask for. A pod up for weeks holds far
	// more, and sending all of it is not a request anybody makes on purpose.
	maxLogTail = 5000
)

// maxReplicas bounds a scale request. Nothing in a home lab wants more, and the
// cap is what stops a mistyped number from asking the scheduler for thousands of
// pods before anyone notices.
const maxReplicas = 100

// validateKind rejects anything that is not one of the three kinds reported.
func validateKind(kind string) error {
	switch kind {
	case KindDeployment, KindStatefulSet, KindDaemonSet:
		return nil
	default:
		return fmt.Errorf("%w: kind must be %s, %s, or %s",
			ErrInvalidName, KindDeployment, KindStatefulSet, KindDaemonSet)
	}
}

// validateName rejects a resource name Kubernetes would not accept.
//
// The subdomain rule, not the label rule a namespace gets. Pods and workloads are
// DNS-1123 *subdomains*: dots are legal and the limit is 253, and a Deployment
// whose name is long enough that its pods run past 63 characters is perfectly
// ordinary. Validating these as labels would refuse real pods — with a message
// blaming the caller for a rule that does not apply to them.
//
// It takes the parameter's name so the message says which one was wrong, which is
// the whole reason for validating here rather than letting the API server refuse.
func validateName(name, what string) error {
	if len(name) > maxSubdomainLength {
		return fmt.Errorf("%w: %s names may be at most %d characters",
			ErrInvalidName, what, maxSubdomainLength)
	}
	if !dns1123Subdomain.MatchString(name) {
		return fmt.Errorf(
			"%w: %s names must be lowercase letters, digits, hyphens, and dots, "+
				"starting and ending with a letter or digit",
			ErrInvalidName, what)
	}
	return nil
}

// validateLabelName rejects a name that must be a DNS-1123 label rather than a
// subdomain — a container, whose name Kubernetes holds to the stricter rule.
func validateLabelName(name, what string) error {
	if len(name) > maxNamespaceLength {
		return fmt.Errorf("%w: %s names may be at most %d characters",
			ErrInvalidName, what, maxNamespaceLength)
	}
	if !dns1123Label.MatchString(name) {
		return fmt.Errorf(
			"%w: %s names must be lowercase letters, digits, and hyphens, "+
				"starting and ending with a letter or digit",
			ErrInvalidName, what)
	}
	return nil
}

// validateNamespace rejects a name Kubernetes would not accept.
func validateNamespace(name string) error {
	if len(name) > maxNamespaceLength {
		return fmt.Errorf("%w: namespace names may be at most %d characters",
			ErrInvalidName, maxNamespaceLength)
	}
	if !dns1123Label.MatchString(name) {
		return fmt.Errorf(
			"%w: namespace names must be lowercase letters, digits, and hyphens, "+
				"starting and ending with a letter or digit",
			ErrInvalidName)
	}
	return nil
}
