package helm

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/eetr-ai/home-lab/admin/api/internal/nspolicy"
)

// fakeRepo records what reached the storage, so a test can assert that a refusal
// happened before anything did.
type fakeRepo struct {
	asked []string
	// offered is what a chart repository answers with, and versionErr is what it
	// answers with instead when it cannot be reached.
	offered    []ChartVersion
	versionErr error
	// fetches counts trips to the repository, which is how the cache is tested
	// without waiting for anything.
	fetches int
}

func (f *fakeRepo) ListChartVersions(_ context.Context, source ChartSource) ([]ChartVersion, error) {
	f.fetches++
	f.asked = append(f.asked, "versions:"+source.Chart)
	if f.versionErr != nil {
		return nil, f.versionErr
	}
	return f.offered, nil
}

func (f *fakeRepo) ListReleases(_ context.Context, namespaces []string) ([]Release, error) {
	f.asked = append(f.asked, "list:"+strings.Join(namespaces, ","))
	return []Release{{Name: "whoami", Namespace: namespaces[0]}}, nil
}

func (f *fakeRepo) ReadRelease(_ context.Context, namespace, name string) (ReleaseDetail, error) {
	f.asked = append(f.asked, "read:"+namespace+"/"+name)
	return ReleaseDetail{Release: Release{Name: name, Namespace: namespace}}, nil
}

func (f *fakeRepo) ReadHistory(_ context.Context, namespace, name string) ([]Revision, error) {
	f.asked = append(f.asked, "history:"+namespace+"/"+name)
	return []Revision{{Revision: 1}}, nil
}

// newTestService builds a service with the policy this lab actually runs: the
// panel in "admin", platform-system protected, and "apps" the one Helm target.
func newTestService(repo repository) *Service {
	return newTestServiceWithCatalog(repo, Catalog{})
}

func newTestServiceWithCatalog(repo repository, catalog Catalog) *Service {
	return NewService(repo, nspolicy.New(nspolicy.Config{
		Own:       "admin",
		Protected: []string{"platform-system"},
		Managed:   []string{"apps"},
	}), catalog, slog.New(slog.DiscardHandler))
}

// Every read is refused for a namespace this slice may not reach, and refused
// before the storage is touched. The three routes have to agree, or the one that
// forgets becomes the way in.
func TestReadsRefuseANamespaceThisSliceMayNotReach(t *testing.T) {
	reads := map[string]func(*Service, string) error{
		"list": func(s *Service, namespace string) error {
			_, err := s.ListNamespaceReleases(t.Context(), namespace)
			return err
		},
		"read": func(s *Service, namespace string) error {
			_, err := s.ReadRelease(t.Context(), namespace, "whoami")
			return err
		},
		"history": func(s *Service, namespace string) error {
			_, err := s.ReadHistory(t.Context(), namespace, "whoami")
			return err
		},
	}

	tests := []struct {
		name      string
		namespace string
		wantErr   error
	}{
		{
			name:      "a managed namespace",
			namespace: "apps",
		},
		{
			// Permanent, and reported as such: platform-system will never be a
			// Helm target from here however it is configured.
			name:      "a namespace protected by configuration",
			namespace: "platform-system",
			wantErr:   ErrProtected,
		},
		{
			name:      "a Kubernetes system namespace",
			namespace: "kube-system",
			wantErr:   ErrProtected,
		},
		{
			name:      "the panel's own namespace",
			namespace: "admin",
			wantErr:   ErrProtected,
		},
		{
			// Changeable, and distinguished from protected for that reason: this
			// one is a values file away from working.
			name:      "a namespace nobody configured",
			namespace: "other",
			wantErr:   ErrUnmanaged,
		},
		{
			name:      "a name Kubernetes would not accept",
			namespace: "Apps_1",
			wantErr:   ErrInvalidName,
		},
	}

	for name, read := range reads {
		for _, test := range tests {
			t.Run(name+"/"+test.name, func(t *testing.T) {
				repo := &fakeRepo{}
				err := read(newTestService(repo), test.namespace)

				if !errors.Is(err, test.wantErr) {
					t.Fatalf("error = %v, want %v", err, test.wantErr)
				}
				if test.wantErr != nil && len(repo.asked) != 0 {
					t.Fatalf("a refused read still reached the storage: %v", repo.asked)
				}
				if test.wantErr == nil && len(repo.asked) != 1 {
					t.Fatalf("the storage was asked %d times, want 1", len(repo.asked))
				}
			})
		}
	}
}

func TestReleaseNameValidation(t *testing.T) {
	tests := []struct {
		name    string
		release string
		wantErr error
	}{
		{name: "an ordinary name", release: "whoami"},
		{name: "hyphens and digits", release: "my-app-2"},
		{name: "empty", release: "", wantErr: ErrInvalidName},
		{name: "uppercase", release: "MyApp", wantErr: ErrInvalidName},
		{name: "a leading hyphen", release: "-app", wantErr: ErrInvalidName},
		{
			// Helm's own limit, and it is 53 rather than 63 because Helm appends
			// to the name when it names what a chart creates. Letting Helm refuse
			// it would be a 500 carrying an internal message.
			name:    "longer than Helm allows",
			release: strings.Repeat("a", maxReleaseNameLength+1),
			wantErr: ErrInvalidName,
		},
		{
			name:    "exactly as long as Helm allows",
			release: strings.Repeat("a", maxReleaseNameLength),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepo{}
			_, err := newTestService(repo).ReadRelease(t.Context(), "apps", test.release)

			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr != nil && len(repo.asked) != 0 {
				t.Fatalf("a refused read still reached the storage: %v", repo.asked)
			}
		})
	}
}

// With nothing configured there is no namespace to look in, and that is not the
// same as there being no releases. Answering with an empty list would report a
// lab that was never switched on as a lab with nothing installed.
func TestListReleasesReportsAnUnconfiguredLab(t *testing.T) {
	repo := &fakeRepo{}
	service := NewService(repo, nspolicy.New(nspolicy.Config{Own: "admin"}), Catalog{},
		slog.New(slog.DiscardHandler))

	_, err := service.ListReleases(t.Context())
	if !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("error = %v, want %v", err, ErrNotConfigured)
	}
	if len(repo.asked) != 0 {
		t.Fatalf("an unconfigured lab still reached the storage: %v", repo.asked)
	}
}

// The cluster-wide listing asks for exactly the configured namespaces. Reading
// every namespace instead would need a cluster-wide grant on Helm's release
// Secrets, which is the one thing this design refuses to hold.
func TestListReleasesAsksOnlyForConfiguredNamespaces(t *testing.T) {
	repo := &fakeRepo{}
	if _, err := newTestService(repo).ListReleases(t.Context()); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(repo.asked) != 1 || repo.asked[0] != "list:apps" {
		t.Errorf("asked = %v, want a single listing of the configured namespaces", repo.asked)
	}
}
