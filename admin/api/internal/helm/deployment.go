package helm

import "time"

// Sources a version can have been written by.
const (
	// SourcePanel is an operator editing values in the panel.
	SourcePanel = "panel"
	// SourceCI is a pipeline posting a version and overrides.
	SourceCI = "ci"
)

// The states a deployment can be in, which is the whole reason for holding a
// record separate from the cluster: the two can disagree, and the panel has to
// say so rather than picking one and pretending.
const (
	// StateInSync means the newest version is what the cluster is running.
	StateInSync = "in-sync"
	// StatePending means the newest version has not been rolled out yet.
	StatePending = "pending"
	// StateDrifted means the cluster is running something other than the newest
	// rolled-out version — somebody changed the release outside the panel.
	StateDrifted = "drifted"
	// StateNotInstalled means this deployment has a record and no release.
	StateNotInstalled = "not-installed"
	// StateUnknown means the live release could not be read, so nothing can be
	// said about whether the record matches it. Distinct from not-installed on
	// purpose: reporting a failed read as "not installed" would invite an
	// operator to install a second copy of something already running.
	StateUnknown = "unknown"
)

// Deployment is a chart this lab has declared for a namespace.
//
// It is desired state. Nothing on it is read from the cluster, and it says
// nothing about whether the release exists or is healthy — DeploymentDetail
// carries that, read live from Helm at the moment it is asked for.
type Deployment struct {
	ID          string `json:"id"`
	Namespace   string `json:"namespace"`
	ReleaseName string `json:"releaseName"`
	// ChartRef is the reference an operator typed, and the thing a new version
	// is fetched from. It never carries credentials.
	ChartRef  string    `json:"chartRef"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
}

// DeploymentVersion is one (chart version, values) pair that was declared.
//
// Versions are append-only and number from 1. A version that was never rolled
// out still exists, which is what makes editing values and deploying them two
// separate decisions.
type DeploymentVersion struct {
	Version      int    `json:"version"`
	ChartVersion string `json:"chartVersion"`
	// ValuesYAML is the document as it was written, comments and all.
	ValuesYAML string `json:"valuesYaml"`
	// Source is SourcePanel or SourceCI.
	Source    string    `json:"source"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
	// RolledOutAt is nil until this version reached the cluster.
	RolledOutAt *time.Time `json:"rolledOutAt,omitempty"`
	// HelmRevision lines this version up against Helm's own history.
	HelmRevision *int `json:"helmRevision,omitempty"`
}

// DeploymentSummary is a deployment as a list shows it.
type DeploymentSummary struct {
	Deployment
	// Current is the newest declared version, which is what a rollout would
	// apply.
	Current DeploymentVersion `json:"current"`
	// Status is Helm's own word for the live release, empty when there is none.
	Status string `json:"status,omitempty"`
	// State is this record measured against the cluster — see the State
	// constants.
	State string `json:"state"`
}

// DeploymentDetail is a deployment with its live release beside it.
type DeploymentDetail struct {
	DeploymentSummary
	// Release is what Helm has, and nil when the release is not installed or
	// could not be read.
	Release *ReleaseDetail `json:"release,omitempty"`
	// ReleaseError says why the live release could not be read, when it could
	// not. Reported rather than swallowed: a page that silently shows a record
	// with no release beside it is a page that says "nothing is deployed" when
	// what happened was a refused Secret read.
	ReleaseError string `json:"releaseError,omitempty"`
	// Versions are every declared version, newest first.
	Versions []DeploymentVersion `json:"versions"`
}

// describeState works out how a record stands against the cluster.
//
// The rules, in the order they are asked:
//
//   - the live release could not be read at all — unknown, and say why
//   - there is no release — not installed
//   - the newest version has never been rolled out — pending
//   - the release is running a different chart version from the newest rolled-out
//     one — drifted
//   - otherwise in sync
//
// Drift is measured against chart version rather than against values, because
// Helm stores the values it was given and comparing two YAML documents that mean
// the same thing would report drift for a reordered key.
func describeState(current DeploymentVersion, release *Release, readFailed bool) string {
	switch {
	case readFailed:
		return StateUnknown
	case release == nil:
		return StateNotInstalled
	case current.RolledOutAt == nil:
		return StatePending
	case release.ChartVersion != "" && release.ChartVersion != current.ChartVersion:
		return StateDrifted
	default:
		return StateInSync
	}
}
