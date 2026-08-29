package helm

import (
	"fmt"
	"regexp"
)

// exactVersion is a version and not a range.
//
// Helm accepts a constraint wherever it accepts a version, and "^1.2" would mean
// this lab installs whatever satisfies it on the day it happens to run. That is
// the opposite of what pinning is for, and this repository pins everything —
// providers, actions, charts, images. So a constraint is refused rather than
// resolved, and so is "latest".
var exactVersion = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// installSpec is everything the repository needs to put a release on the cluster.
//
// Not a request body. The chart has already been resolved to a source and the
// values already parsed by the time this exists, so nothing here came straight
// off the wire.
type installSpec struct {
	Namespace         string
	Name              string
	Source            ChartSource
	Version           string
	Values            map[string]any
	RollbackOnFailure bool
}

// upgradeSpec is the same for moving a release to another version.
type upgradeSpec struct {
	Namespace         string
	Name              string
	Source            ChartSource
	Version           string
	Values            map[string]any
	RollbackOnFailure bool
}

// RollbackRequest is a request to return a release to an earlier revision.
type RollbackRequest struct {
	// Revision is required. Helm defaults a missing one to "the previous
	// revision", which is a different operation from the one an operator clicked
	// in a table of revisions, and defaulting to it would sometimes do the right
	// thing quietly and sometimes the wrong thing quietly.
	Revision int `json:"revision"`
}

// Accepted is what a mutation answers with.
//
// The operation was accepted, not performed: Helm waits for pods and that
// outlasts any HTTP request this API is willing to hold open. The outcome is read
// back from Helm's storage, which is the same place both replicas read it from.
type Accepted struct {
	Namespace string `json:"namespace"`
	Release   string `json:"release"`
	// Operation is install, upgrade, rollback, or uninstall.
	Operation string `json:"operation"`
	// Message says how to find out what happened, because the record of what was
	// asked for and the record of what happened are two different systems.
	Message string `json:"message"`
}

// validateVersion refuses anything that is not one exact version.
func validateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("%w: a version is required", ErrInvalidName)
	}
	if !exactVersion.MatchString(version) {
		return fmt.Errorf(
			"%w: %q is not an exact version — this lab pins versions, so a range "+
				"or \"latest\" is refused rather than resolved",
			ErrInvalidName, version)
	}
	return nil
}
