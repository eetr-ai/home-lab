package helm

import (
	"fmt"
	"regexp"
)

// maxReleaseNameLength is Helm's own limit.
//
// 53 rather than 63, because Helm appends to the release name when it names the
// resources a chart creates, and the result still has to be a DNS label. Checking
// it here turns a 500 carrying Helm's internal message into a 400 that says what
// was wrong.
const maxReleaseNameLength = 53

// maxNamespaceLength is Kubernetes' limit for a DNS-1123 label.
const maxNamespaceLength = 63

// maxJobNameLength is the same limit, applied to a generated Job name.
const maxJobNameLength = 63

// dns1123Label is what both a release name and a namespace name must look like.
var dns1123Label = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// validateReleaseName rejects a name Helm would not accept.
func validateReleaseName(name string) error {
	if len(name) > maxReleaseNameLength {
		return fmt.Errorf("%w: release names may be at most %d characters",
			ErrInvalidName, maxReleaseNameLength)
	}
	if !dns1123Label.MatchString(name) {
		return fmt.Errorf(
			"%w: release names must be lowercase letters, digits, and hyphens, "+
				"starting and ending with a letter or digit",
			ErrInvalidName)
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

// validateJobName rejects a name the API server would not have generated.
//
// Separate from validateNamespace, which it used to borrow. The two happen to
// share a rule today -- both are DNS-1123 labels -- and borrowing one for the
// other means a change to what a namespace may be called silently changes what a
// job may be called, in a message that then talks about namespaces.
func validateJobName(name string) error {
	if len(name) > maxJobNameLength {
		return fmt.Errorf("%w: job names may be at most %d characters",
			ErrInvalidName, maxJobNameLength)
	}
	if !dns1123Label.MatchString(name) {
		return fmt.Errorf("%w: %q is not a job name", ErrInvalidName, name)
	}
	return nil
}
