package helm

import (
	"errors"
	"slices"
	"testing"
)

func TestCatalogValidate(t *testing.T) {
	traefik := Repo{Name: "traefik", URL: "https://traefik.github.io/charts"}

	tests := []struct {
		name    string
		catalog Catalog
		wantErr bool
	}{
		{
			name: "a usable catalog",
			catalog: Catalog{
				Repositories: []Repo{traefik},
				Charts:       []Chart{{Name: "whoami", Chart: "whoami", Repository: "traefik"}},
			},
		},
		{
			name:    "an empty catalog, which is a lab that has not switched this on",
			catalog: Catalog{},
		},
		{
			name:    "a repository with no url",
			catalog: Catalog{Repositories: []Repo{{Name: "traefik"}}},
			wantErr: true,
		},
		{
			name:    "a repository with no name",
			catalog: Catalog{Repositories: []Repo{{URL: "https://example.invalid"}}},
			wantErr: true,
		},
		{
			name: "two repositories with the same name",
			catalog: Catalog{Repositories: []Repo{
				traefik,
				{Name: "traefik", URL: "https://elsewhere.invalid"},
			}},
			wantErr: true,
		},
		{
			name: "two charts with the same name",
			catalog: Catalog{
				Repositories: []Repo{traefik},
				Charts: []Chart{
					{Name: "whoami", Chart: "whoami", Repository: "traefik"},
					{Name: "whoami", Chart: "other", Repository: "traefik"},
				},
			},
			wantErr: true,
		},
		{
			// The mistake that is otherwise silent: the entry shows up in the
			// panel and cannot be installed, because of a name that does not match
			// one written a few lines higher.
			name: "a chart naming a repository nobody declared",
			catalog: Catalog{
				Repositories: []Repo{traefik},
				Charts:       []Chart{{Name: "redis", Chart: "redis", Repository: "bitnami"}},
			},
			wantErr: true,
		},
		{
			name: "a chart with no chart name",
			catalog: Catalog{
				Repositories: []Repo{traefik},
				Charts:       []Chart{{Name: "whoami", Repository: "traefik"}},
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.catalog.validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("validate() = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestCatalogFind(t *testing.T) {
	catalog := Catalog{
		Repositories: []Repo{
			{Name: "traefik", URL: "https://traefik.github.io/charts"},
			{Name: "internal", URL: "oci://registry.invalid/charts", OCI: true},
		},
		Charts: []Chart{
			{Name: "whoami", Chart: "whoami", Repository: "traefik"},
			{Name: "panel", Chart: "home-lab-admin", Repository: "internal"},
		},
	}

	// The catalog key and the chart's own name are separate on purpose: two
	// repositories may both publish a "redis".
	chart, source, err := catalog.Find("panel")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if chart.Chart != "home-lab-admin" || source.Chart != "home-lab-admin" {
		t.Errorf("source = %+v, want the chart's own name rather than the catalog key", source)
	}
	if !source.OCI || source.URL != "oci://registry.invalid/charts" {
		t.Errorf("source = %+v, want the declaring repository's url and kind", source)
	}

	if _, _, err := catalog.Find("nothing"); !errors.Is(err, ErrUnknownChart) {
		t.Errorf("find(unknown) = %v, want %v", err, ErrUnknownChart)
	}
}

func TestListChartsNarrowsToWhatThisLabPermits(t *testing.T) {
	offered := []ChartVersion{{Version: "1.0.0"}, {Version: "1.1.0"}, {Version: "2.0.0"}}
	catalog := func(pinned ...string) Catalog {
		return Catalog{
			Repositories: []Repo{{Name: "r", URL: "https://example.invalid"}},
			Charts:       []Chart{{Name: "c", Chart: "c", Repository: "r", Versions: pinned}},
		}
	}

	tests := []struct {
		name   string
		pinned []string
		want   []string
	}{
		{
			name: "no pins offers everything the repository has",
			want: []string{"1.0.0", "1.1.0", "2.0.0"},
		},
		{
			name:   "pins narrow to exactly those",
			pinned: []string{"1.1.0"},
			want:   []string{"1.1.0"},
		},
		{
			// A pin naming a version that was yanked must not be installable. The
			// repository's list is consulted even when the catalog pins, or the
			// panel would offer a version that cannot be fetched.
			name:   "a pin the repository no longer offers is dropped",
			pinned: []string{"1.1.0", "9.9.9"},
			want:   []string{"1.1.0"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepo{offered: offered}
			listings, err := newTestServiceWithCatalog(repo, catalog(test.pinned...)).
				ListCharts(t.Context())
			if err != nil {
				t.Fatalf("list: %v", err)
			}

			got := versionStrings(listings[0].Versions)
			if !slices.Equal(got, test.want) {
				t.Errorf("versions = %v, want %v", got, test.want)
			}
			if listings[0].Unavailable {
				t.Error("a reachable repository was reported unavailable")
			}
		})
	}
}

// An unreachable repository degrades to what configuration alone knows rather
// than failing. That is the whole point of the catalog being configuration: the
// pinned versions are the ones an operator was going to pick anyway.
func TestListChartsDegradesWhenARepositoryIsUnreachable(t *testing.T) {
	catalog := Catalog{
		Repositories: []Repo{{Name: "r", URL: "https://example.invalid"}},
		Charts: []Chart{
			{Name: "pinned", Chart: "a", Repository: "r", Versions: []string{"1.0.0"}},
			{Name: "open", Chart: "b", Repository: "r"},
		},
	}
	repo := &fakeRepo{versionErr: ErrRepositoryUnreachable}

	listings, err := newTestServiceWithCatalog(repo, catalog).ListCharts(t.Context())
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	for _, listing := range listings {
		if !listing.Unavailable {
			t.Errorf("%s was not marked unavailable", listing.Name)
		}
		if !listing.FetchedAt.IsZero() {
			t.Errorf("%s reported a fetch time for a fetch that failed", listing.Name)
		}
	}
	if got := versionStrings(listings[0].Versions); !slices.Equal(got, []string{"1.0.0"}) {
		t.Errorf("pinned entry offered %v, want its pinned versions", got)
	}
	if got := listings[1].Versions; len(got) != 0 {
		t.Errorf("unpinned entry offered %v, want nothing it cannot confirm", got)
	}
}

// The catalog is what makes installing bounded, so an empty one is reported as
// unconfigured rather than as an empty list -- a lab that was never switched on
// is not a lab with no charts.
func TestChartRoutesReportAnUnconfiguredLab(t *testing.T) {
	service := newTestServiceWithCatalog(&fakeRepo{}, Catalog{})

	if _, err := service.ListCharts(t.Context()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("ListCharts = %v, want %v", err, ErrNotConfigured)
	}
	if _, err := service.ListChartVersions(t.Context(), "c"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("ListChartVersions = %v, want %v", err, ErrNotConfigured)
	}
}

// A repository is asked once and reused, or browsing the catalog is a request to
// a third party per click for an answer that changed days ago.
func TestChartVersionsAreCached(t *testing.T) {
	catalog := Catalog{
		Repositories: []Repo{{Name: "r", URL: "https://example.invalid"}},
		Charts:       []Chart{{Name: "c", Chart: "c", Repository: "r"}},
	}
	repo := &fakeRepo{offered: []ChartVersion{{Version: "1.0.0"}}}
	service := newTestServiceWithCatalog(repo, catalog)

	for range 3 {
		if _, err := service.ListCharts(t.Context()); err != nil {
			t.Fatalf("list: %v", err)
		}
	}
	if repo.fetches != 1 {
		t.Errorf("the repository was read %d times, want 1", repo.fetches)
	}
}

// A failed fetch is not cached. Caching it would leave the panel reporting a
// repository as unavailable for the whole TTL after it came back.
func TestAFailedFetchIsNotCached(t *testing.T) {
	catalog := Catalog{
		Repositories: []Repo{{Name: "r", URL: "https://example.invalid"}},
		Charts:       []Chart{{Name: "c", Chart: "c", Repository: "r"}},
	}
	repo := &fakeRepo{versionErr: ErrRepositoryUnreachable}
	service := newTestServiceWithCatalog(repo, catalog)

	for range 2 {
		if _, err := service.ListCharts(t.Context()); err != nil {
			t.Fatalf("list: %v", err)
		}
	}
	if repo.fetches != 2 {
		t.Errorf("the repository was read %d times, want it retried rather than cached", repo.fetches)
	}
}

func versionStrings(versions []ChartVersion) []string {
	names := make([]string, 0, len(versions))
	for _, version := range versions {
		names = append(names, version.Version)
	}
	return names
}
