package helm

import "time"

// Release is one Helm release as the panel shows it.
//
// Every field comes from Helm's own storage, which is the source of truth here:
// there is no database behind this slice, and a release's identity, values, and
// history live in Secrets in the namespace it was installed into. That is what
// lets both API replicas agree, and what makes a release installed by hand
// visible here without anything having recorded it.
type Release struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	// Revision counts up from 1 and never repeats, including across a rollback:
	// rolling back to revision 2 creates revision 5 rather than returning to 2.
	Revision int `json:"revision"`
	// Status is Helm's own vocabulary — deployed, failed, pending-upgrade,
	// superseded, uninstalling — passed through rather than mapped, because an
	// operator reading it here and running `helm status` should see one word.
	Status       string `json:"status"`
	Chart        string `json:"chart"`
	ChartVersion string `json:"chartVersion"`
	AppVersion   string `json:"appVersion"`
	// Description is the log entry Helm writes on each revision: "Upgrade
	// complete", or the reason one failed. It is the most useful field on a
	// broken release and the least interesting on a healthy one.
	Description string    `json:"description,omitempty"`
	Updated     time.Time `json:"updatedAt"`
}

// ReleaseDetail is one release with what it was configured with.
type ReleaseDetail struct {
	Release
	// Values are the values supplied when the release was installed or upgraded,
	// and not the coalesced set. The coalesced set is the chart's entire
	// values.yaml merged with these, which an operator would then re-submit as
	// their own — pinning every default the chart ships with, forever.
	Values map[string]any `json:"values"`
	// Notes is the chart's rendered NOTES.txt, which is where a chart says how to
	// reach what it just installed.
	Notes string `json:"notes,omitempty"`
}

// Revision is one entry in a release's history.
type Revision struct {
	Revision     int       `json:"revision"`
	Status       string    `json:"status"`
	ChartVersion string    `json:"chartVersion"`
	AppVersion   string    `json:"appVersion"`
	Description  string    `json:"description,omitempty"`
	Updated      time.Time `json:"updatedAt"`
}
