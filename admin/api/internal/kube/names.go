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
