package helm

import (
	"context"
	"sync"
	"time"
)

// versionCacheTTL is how long a repository's version list is reused.
//
// A chart repository publishes a new version rarely and this panel asks on every
// page load, so refetching each time would be a request to a third party per
// click for an answer that changed days ago. Five minutes is short enough that a
// version published while an operator is working shows up within one coffee, and
// long enough that browsing the catalog costs nothing.
const versionCacheTTL = 5 * time.Minute

// versionCache remembers what a repository offered, and when it said so.
//
// In-process, and the API runs two replicas, so the two can disagree for up to
// the TTL. That is fine here and would not be for anything that decides: this
// only decides what to *offer*, and an install still checks the version against
// the repository at the moment it runs.
type versionCache struct {
	mu      sync.Mutex
	entries map[string]cachedVersions
	now     func() time.Time
}

type cachedVersions struct {
	versions []ChartVersion
	fetched  time.Time
}

func newVersionCache() *versionCache {
	return &versionCache{entries: map[string]cachedVersions{}, now: time.Now}
}

// get returns a cached list and when it was fetched, if it is still fresh.
func (c *versionCache) get(key string) ([]ChartVersion, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok || c.now().Sub(entry.fetched) > versionCacheTTL {
		return nil, time.Time{}, false
	}
	return entry.versions, entry.fetched, true
}

func (c *versionCache) put(key string, versions []ChartVersion) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	fetched := c.now()
	c.entries[key] = cachedVersions{versions: versions, fetched: fetched}
	return fetched
}

// ChartListing is one catalogue entry with what can actually be installed.
//
// The catalog fields are repeated rather than embedded. Embedding Chart would put
// two `versions` fields in one struct -- the entry's pinned strings and the
// resolved list -- and rely on Go's shadowing rules to decide which one is
// serialised. That works and it is invisible: a reader of the JSON has no way to
// tell which one they are looking at, and moving a field would silently change
// the wire format.
type ChartListing struct {
	Name        string `json:"name"`
	Chart       string `json:"chart"`
	Repository  string `json:"repository"`
	Description string `json:"description,omitempty"`
	// Versions is what this lab permits and the repository offers, which is the
	// intersection rather than either one.
	Versions []ChartVersion `json:"versions"`
	// FetchedAt is when the version list was read from the repository. Reported
	// because the list is cached, and a stale answer is only a problem when
	// nobody can tell it is stale.
	FetchedAt time.Time `json:"fetchedAt,omitzero"`
	// Unavailable says the repository could not be reached, and that Versions
	// therefore holds what configuration alone knows — the pinned list, or
	// nothing. The catalog degrading to its own declarations is the point of not
	// having a database.
	Unavailable bool `json:"unavailable,omitempty"`
}

// ListCharts returns the catalog with each entry's installable versions.
func (s *Service) ListCharts(ctx context.Context) ([]ChartListing, error) {
	if !s.catalog.Configured() {
		return nil, ErrNotConfigured
	}

	listings := make([]ChartListing, 0, len(s.catalog.Charts))
	for _, chart := range s.catalog.Charts {
		listings = append(listings, s.listChart(ctx, chart))
	}
	return listings, nil
}

// ListChartVersions returns one catalogue entry's installable versions.
func (s *Service) ListChartVersions(ctx context.Context, name string) (ChartListing, error) {
	if !s.catalog.Configured() {
		return ChartListing{}, ErrNotConfigured
	}

	chart, _, err := s.catalog.Find(name)
	if err != nil {
		return ChartListing{}, err
	}
	return s.listChart(ctx, chart), nil
}

// listChart resolves one entry, falling back to configuration when the
// repository cannot be reached.
//
// An unreachable repository is not an error here. The catalog still knows what it
// declared, and a panel that shows the pinned versions with a warning is more
// useful than one that shows a failure — especially since the pinned versions are
// the ones an operator was going to pick anyway.
func (s *Service) listChart(ctx context.Context, chart Chart) ChartListing {
	_, source, err := s.catalog.Find(chart.Name)
	if err != nil {
		return listingOf(chart, []ChartVersion{}, time.Time{}, true)
	}

	if versions, fetched, ok := s.versions.get(chart.Name); ok {
		return listingOf(chart, permitted(chart, versions), fetched, false)
	}

	offered, err := s.repo.ListChartVersions(ctx, source)
	if err != nil {
		s.logger.Warn("could not read a chart repository",
			"chart", chart.Name, "repository", chart.Repository, "error", err)
		return listingOf(chart, declaredOnly(chart), time.Time{}, true)
	}

	fetched := s.versions.put(chart.Name, offered)
	return listingOf(chart, permitted(chart, offered), fetched, false)
}

// listingOf copies a catalogue entry's fields onto a listing.
func listingOf(chart Chart, versions []ChartVersion, fetched time.Time, unavailable bool) ChartListing {
	return ChartListing{
		Name:        chart.Name,
		Chart:       chart.Chart,
		Repository:  chart.Repository,
		Description: chart.Description,
		Versions:    versions,
		FetchedAt:   fetched,
		Unavailable: unavailable,
	}
}

// permitted narrows what a repository offers to what this lab allows, newest
// first — which is the order an operator picks from.
func permitted(chart Chart, offered []ChartVersion) []ChartVersion {
	allowed := make([]ChartVersion, 0, len(offered))
	for _, version := range offered {
		if chart.permits(version.Version, offered) {
			allowed = append(allowed, version)
		}
	}
	return allowed
}

// declaredOnly is what the catalog can offer with no repository to ask: the
// pinned versions, or nothing when the entry pins none.
func declaredOnly(chart Chart) []ChartVersion {
	versions := make([]ChartVersion, 0, len(chart.Versions))
	for _, version := range chart.Versions {
		versions = append(versions, ChartVersion{Version: version})
	}
	return versions
}
