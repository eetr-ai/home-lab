package helm

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/eetr-ai/home-lab/admin/api/internal/nspolicy"
)

// fakeRepo records what reached the storage, so a test can assert that a refusal
// happened before anything did.
type fakeRepo struct {
	asked []string
	// ran carries each completed operation, so a test can wait for the detached
	// job instead of sleeping and hoping.
	ran chan string
	// readErr, when set, is what ReadRelease answers with.
	readErr error
	// versions is what the chart repository is pretending to offer.
	versions []ChartVersion
	// versionsErr, when set, stands in for an unreachable registry.
	versionsErr error
	// installed records the spec the repository was asked to install.
	installed installSpec
	// upgraded records the spec the repository was asked to upgrade to.
	upgraded upgradeSpec
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{ran: make(chan string, 8)}
}

func (f *fakeRepo) ListReleases(_ context.Context, namespaces []string) ([]Release, error) {
	f.asked = append(f.asked, "list:"+strings.Join(namespaces, ","))
	return []Release{{Name: "whoami", Namespace: namespaces[0]}}, nil
}

func (f *fakeRepo) ReadRelease(_ context.Context, namespace, name string) (ReleaseDetail, error) {
	f.asked = append(f.asked, "read:"+namespace+"/"+name)
	if f.readErr != nil {
		return ReleaseDetail{}, f.readErr
	}
	return ReleaseDetail{Release: Release{Name: name, Namespace: namespace}}, nil
}

func (f *fakeRepo) ListChartVersions(_ context.Context, source ChartSource) ([]ChartVersion, error) {
	f.asked = append(f.asked, "versions:"+source.Ref())
	return f.versions, f.versionsErr
}

func (f *fakeRepo) Install(_ context.Context, spec installSpec) (Release, error) {
	f.installed = spec
	f.asked = append(f.asked, "install:"+spec.Namespace+"/"+spec.Name)
	f.finished("install")
	return Release{Name: spec.Name, Namespace: spec.Namespace, Revision: 1}, nil
}

func (f *fakeRepo) Upgrade(_ context.Context, spec upgradeSpec) (Release, error) {
	f.upgraded = spec
	f.asked = append(f.asked, "upgrade:"+spec.Namespace+"/"+spec.Name)
	f.finished("upgrade")
	return Release{Name: spec.Name, Namespace: spec.Namespace, Revision: 2}, nil
}

func (f *fakeRepo) Rollback(_ context.Context, namespace, name string, revision int) error {
	f.asked = append(f.asked, "rollback:"+namespace+"/"+name)
	_ = revision
	f.finished("rollback")
	return nil
}

func (f *fakeRepo) Uninstall(_ context.Context, namespace, name string) error {
	f.asked = append(f.asked, "uninstall:"+namespace+"/"+name)
	f.finished("uninstall")
	return nil
}

func (f *fakeRepo) finished(operation string) {
	if f.ran != nil {
		f.ran <- operation
	}
}

func (f *fakeRepo) ReadHistory(_ context.Context, namespace, name string) ([]Revision, error) {
	f.asked = append(f.asked, "history:"+namespace+"/"+name)
	return []Revision{{Revision: 1}}, nil
}

// newTestService builds a service with the policy this lab actually runs: the
// panel in "admin", platform-system protected, and "apps" the one Helm target.
func newTestService(repo repository) *Service {
	return NewService(repo, nil, testPolicy(), Self{}, time.Minute,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// testPolicy is the policy this lab actually runs: the panel in "admin",
// platform-system protected, and "apps" the one Helm target.
func testPolicy() nspolicy.Policy {
	return nspolicy.New(nspolicy.Config{
		Own:       "admin",
		Protected: []string{"platform-system"},
		Managed:   []string{"apps"},
	})
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
			// Not protected any more, and unmanaged only because testPolicy does
			// not name it. A lab that names it may deploy the panel from a
			// pipeline, which is what this feature was asked for.
			name:      "the panel's own namespace",
			namespace: "admin",
			wantErr:   ErrUnmanaged,
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
				repo := newFakeRepo()
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
			repo := newFakeRepo()
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
	repo := newFakeRepo()
	service := NewService(repo, nil, nspolicy.New(nspolicy.Config{Own: "admin"}), Self{}, time.Minute,
		slog.New(slog.NewTextHandler(io.Discard, nil)))

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
	repo := newFakeRepo()
	if _, err := newTestService(repo).ListReleases(t.Context()); err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(repo.asked) != 1 || repo.asked[0] != "list:apps" {
		t.Errorf("asked = %v, want a single listing of the configured namespaces", repo.asked)
	}
}
