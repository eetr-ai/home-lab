package helm

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// maxValuesBytes bounds the values a request may carry.
//
// Values end up in a Secret, and a Secret is capped at roughly a megabyte by
// etcd. A request over that limit fails at the moment Helm writes the release,
// which is after the chart has already been applied to the cluster — so the
// refusal has to happen here, before anything has been done.
const maxValuesBytes = 256 * 1024

// exactVersion is a version and not a range.
//
// Helm accepts a constraint wherever it accepts a version, and "^1.2" would mean
// this lab installs whatever satisfies it on the day it happens to run. That is
// the opposite of what pinning is for, and this repository pins everything —
// providers, actions, charts, images. So a constraint is refused rather than
// resolved, and so is "latest".
var exactVersion = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// InstallRequest is a request to put a new release on the cluster.
type InstallRequest struct {
	Namespace string         `json:"-"`
	Name      string         `json:"name"`
	Chart     string         `json:"chart"`
	Version   string         `json:"version"`
	Values    map[string]any `json:"values,omitempty"`
	// RollbackOnFailure asks Helm to undo a failed install rather than leaving it
	// failed. Off by default, deliberately — see UpgradeRequest.
	RollbackOnFailure bool `json:"rollbackOnFailure,omitempty"`
}

// UpgradeRequest is a request to move a release to another version.
//
// Values are optional and their absence is meaningful: it means "keep what this
// release already has", which is what makes this callable from a pipeline that
// owns an image tag and knows nothing about the rest of the configuration.
type UpgradeRequest struct {
	Namespace string         `json:"-"`
	Name      string         `json:"-"`
	Version   string         `json:"version"`
	Values    map[string]any `json:"values,omitempty"`
	// RollbackOnFailure is off by default, and this is the one default worth
	// arguing about. With it on, a failed upgrade is undone and the release ends
	// up `deployed` on a *new* revision — so a pipeline polling for a terminal
	// status reads success for a deploy that did not deploy. Failing loudly and
	// leaving the release `failed` is legible to a human on the release page and
	// to a curl loop in a pipeline, and rolling back is one call away.
	RollbackOnFailure bool `json:"rollbackOnFailure,omitempty"`
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
// back from Helm's storage through the release endpoint, which is the same place
// both replicas read it from.
type Accepted struct {
	Namespace string `json:"namespace"`
	Release   string `json:"release"`
	// Operation is install, upgrade, rollback, or uninstall.
	Operation string `json:"operation"`
	// Message says how to find out what happened, because there is nothing else
	// to poll and no job id to hand back.
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

// validateValues refuses values too large to be stored.
func validateValues(values map[string]any) error {
	if len(values) == 0 {
		return nil
	}

	encoded, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("%w: the values are not encodable", ErrInvalidName)
	}
	if len(encoded) > maxValuesBytes {
		return fmt.Errorf("%w: the values are %d bytes, and at most %d are accepted",
			ErrValuesTooLarge, len(encoded), maxValuesBytes)
	}
	return nil
}
