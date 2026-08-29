package helm

import (
	"fmt"
	"os"
	"slices"

	"sigs.k8s.io/yaml"
)

// Catalog is the set of charts this lab will install, and where they come from.
//
// It is configuration rather than data, and that is the direct consequence of
// having no database: nothing here is discovered, and nothing a caller sends can
// add to it. A request names a catalog entry and a version, never a URL — so the
// API never fetches something a caller chose, which removes a whole class of
// request-forgery and is the first half of what bounds installing arbitrary
// charts.
type Catalog struct {
	Repositories []Repo  `json:"repositories"`
	Charts       []Chart `json:"charts"`
}

// Repo is a chart repository this lab trusts.
type Repo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	// OCI says the URL is a registry rather than an HTTP index. The two are
	// fetched differently and the difference is not inferable from the URL alone,
	// so it is declared.
	OCI bool `json:"oci"`
}

// Chart is one entry an operator may install.
type Chart struct {
	// Name is the catalog key a request uses, and need not be the chart's own
	// name — two repositories may both publish a "redis".
	Name string `json:"name"`
	// Chart is what the chart is called in its repository.
	Chart string `json:"chart"`
	// Repository is the name of one of the repositories above.
	Repository string `json:"repository"`
	// Description is shown in the panel. Free text, and the operator's own words
	// rather than the chart's, since the point is to say why this lab has it.
	Description string `json:"description,omitempty"`
	// Versions, when set, is the exact list this lab permits. Empty means any
	// version the repository offers, which is the looser and more convenient
	// setting and is why it is not the default in the shipped values.
	Versions []string `json:"versions,omitempty"`
}

// ChartVersion is one installable version of a catalogued chart.
type ChartVersion struct {
	Version    string `json:"version"`
	AppVersion string `json:"appVersion,omitempty"`
}

// ChartSource is where one catalogue entry's versions are fetched from.
type ChartSource struct {
	Chart string
	URL   string
	OCI   bool
}

// LoadCatalog reads the catalog from a file.
//
// A file rather than an environment variable, because it is a structure rather
// than a value: it is readable in values.yaml, readable in `kubectl describe`,
// and a checksum over it rolls the pods when it changes. A malformed one is a
// startup failure rather than a runtime one, the same discipline the OIDC issuer
// gets — a panel that starts and then refuses every install is worse than one
// that does not start.
//
// No path means no catalog, which is a valid lab: the release routes still work
// and the chart routes report that nothing was configured.
func LoadCatalog(path string) (Catalog, error) {
	if path == "" {
		return Catalog{}, nil
	}

	// The path is an environment variable set by the chart, read once at startup.
	// It never comes from a request, and there is no point in this process's life
	// at which a caller can influence it.
	content, err := os.ReadFile(path) //nolint:gosec // the path is deployment configuration, not input
	if err != nil {
		return Catalog{}, fmt.Errorf("read the helm catalog at %s: %w", path, err)
	}

	var catalog Catalog
	if err := yaml.Unmarshal(content, &catalog); err != nil {
		return Catalog{}, fmt.Errorf("parse the helm catalog at %s: %w", path, err)
	}
	if err := catalog.validate(); err != nil {
		return Catalog{}, fmt.Errorf("the helm catalog at %s is not usable: %w", path, err)
	}
	return catalog, nil
}

// validate refuses a catalog that would fail later, one entry at a time.
//
// Every one of these is a mistake that otherwise surfaces as a 400 on an install
// somebody was in the middle of, which is both later and further from the values
// file that caused it.
func (c Catalog) validate() error {
	repositories := map[string]bool{}
	for _, repo := range c.Repositories {
		if repo.Name == "" || repo.URL == "" {
			return fmt.Errorf("%w: every repository needs a name and a url", ErrInvalidName)
		}
		if repositories[repo.Name] {
			return fmt.Errorf("%w: two repositories are named %s", ErrInvalidName, repo.Name)
		}
		repositories[repo.Name] = true
	}

	charts := map[string]bool{}
	for _, chart := range c.Charts {
		switch {
		case chart.Name == "" || chart.Chart == "":
			return fmt.Errorf("%w: every chart needs a name and a chart", ErrInvalidName)
		case charts[chart.Name]:
			return fmt.Errorf("%w: two charts are named %s", ErrInvalidName, chart.Name)
		case !repositories[chart.Repository]:
			// The one mistake that is silent otherwise: the entry appears in the
			// panel and cannot be installed, and the reason is a name that does
			// not match one written twelve lines higher up.
			return fmt.Errorf("%w: chart %s names the repository %s, which is not declared",
				ErrInvalidName, chart.Name, chart.Repository)
		}
		charts[chart.Name] = true
	}
	return nil
}

// Configured reports whether this lab has a catalog at all.
func (c Catalog) Configured() bool { return len(c.Charts) > 0 }

// Find returns one catalogue entry and where its versions come from.
func (c Catalog) Find(name string) (Chart, ChartSource, error) {
	for _, chart := range c.Charts {
		if chart.Name != name {
			continue
		}
		for _, repo := range c.Repositories {
			if repo.Name == chart.Repository {
				return chart, ChartSource{Chart: chart.Chart, URL: repo.URL, OCI: repo.OCI}, nil
			}
		}
	}
	return Chart{}, ChartSource{}, fmt.Errorf("%w: %s", ErrUnknownChart, name)
}

// permits reports whether this entry allows a version, given what the repository
// offers.
//
// An entry with no pinned versions permits anything the repository has; one with
// pinned versions permits exactly those. The repository's list is still consulted
// in both cases, because a pin naming a version that was yanked should fail as
// "not offered" rather than being installed from nowhere.
func (chart Chart) permits(version string, offered []ChartVersion) bool {
	if len(chart.Versions) > 0 && !slices.Contains(chart.Versions, version) {
		return false
	}
	return slices.ContainsFunc(offered, func(v ChartVersion) bool { return v.Version == version })
}
