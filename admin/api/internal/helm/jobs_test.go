package helm

import (
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/eetr-ai/home-lab/admin/api/internal/nspolicy"
)

// testCatalog is one vetted chart, pinned to two versions, from an HTTP
// repository.
func testCatalog() Catalog {
	return Catalog{
		Repositories: []Repo{{Name: "r", URL: "https://charts.test.invalid"}},
		Charts: []Chart{{
			Name: "whoami", Chart: "whoami", Repository: "r",
			Versions: []string{"1.0.0", "1.1.0"},
		}},
	}
}

func writeService(repo repository) *Service {
	return NewService(repo, nspolicy.New(nspolicy.Config{
		Own:       "admin",
		Protected: []string{"platform-system"},
		Managed:   []string{"apps"},
	}), testCatalog(), time.Minute, slog.New(slog.DiscardHandler))
}

func newWriteRepo() *fakeRepo {
	return &fakeRepo{
		offered:  []ChartVersion{{Version: "1.0.0"}, {Version: "1.1.0"}, {Version: "2.0.0"}},
		existing: map[string]ReleaseDetail{},
		done:     make(chan string, 4),
	}
}

func TestInstall(t *testing.T) {
	tests := []struct {
		name    string
		request InstallRequest
		wantErr error
	}{
		{
			name:    "a catalogued chart at a permitted version",
			request: InstallRequest{Namespace: "apps", Name: "whoami", Chart: "whoami", Version: "1.1.0"},
		},
		{
			// The allowlist. Without this refusal, installing arbitrary charts is
			// cluster-admin and nothing in the chart can make it otherwise.
			name:    "a chart that is not in the catalog",
			request: InstallRequest{Namespace: "apps", Name: "x", Chart: "postgres", Version: "1.0.0"},
			wantErr: ErrUnknownChart,
		},
		{
			// The repository offers 2.0.0 and this lab does not permit it.
			name:    "a version the catalog does not permit",
			request: InstallRequest{Namespace: "apps", Name: "x", Chart: "whoami", Version: "2.0.0"},
			wantErr: ErrUnknownVersion,
		},
		{
			name:    "a version the repository does not offer",
			request: InstallRequest{Namespace: "apps", Name: "x", Chart: "whoami", Version: "9.9.9"},
			wantErr: ErrUnknownVersion,
		},
		{
			// This repository pins everything it depends on. A constraint would
			// mean installing whatever satisfies it on the day it happens to run.
			name:    "a version range rather than a version",
			request: InstallRequest{Namespace: "apps", Name: "x", Chart: "whoami", Version: "^1.0"},
			wantErr: ErrInvalidName,
		},
		{
			name:    "the word latest",
			request: InstallRequest{Namespace: "apps", Name: "x", Chart: "whoami", Version: "latest"},
			wantErr: ErrInvalidName,
		},
		{
			name:    "no version at all",
			request: InstallRequest{Namespace: "apps", Name: "x", Chart: "whoami"},
			wantErr: ErrInvalidName,
		},
		{
			name:    "a protected namespace",
			request: InstallRequest{Namespace: "platform-system", Name: "x", Chart: "whoami", Version: "1.0.0"},
			wantErr: ErrProtected,
		},
		{
			name:    "a namespace nobody configured",
			request: InstallRequest{Namespace: "other", Name: "x", Chart: "whoami", Version: "1.0.0"},
			wantErr: ErrUnmanaged,
		},
		{
			name: "a release name longer than Helm allows",
			request: InstallRequest{
				Namespace: "apps", Chart: "whoami", Version: "1.0.0",
				Name: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
			wantErr: ErrInvalidName,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newWriteRepo()
			accepted, err := writeService(repo).Install(t.Context(), test.request)

			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr != nil {
				// Nothing may reach the cluster on a refusal. A 202 followed by a
				// silent failure is the failure mode this shape has to avoid.
				repo.mu.Lock()
				installed := len(repo.installed)
				repo.mu.Unlock()
				if installed != 0 {
					t.Fatalf("a refused install still ran")
				}
				return
			}

			if accepted.Operation != "install" {
				t.Errorf("operation = %q, want install", accepted.Operation)
			}
			repo.await(t)
		})
	}
}

// A name already taken is refused before anything runs, so an operator who meant
// to upgrade is told so rather than finding out from Helm.
func TestInstallRefusesANameAlreadyTaken(t *testing.T) {
	repo := newWriteRepo()
	repo.existing["whoami"] = ReleaseDetail{Release: Release{Name: "whoami", Chart: "whoami"}}

	_, err := writeService(repo).Install(t.Context(),
		InstallRequest{Namespace: "apps", Name: "whoami", Chart: "whoami", Version: "1.0.0"})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("error = %v, want %v", err, ErrAlreadyExists)
	}
	if len(repo.installed) != 0 {
		t.Fatal("a refused install still ran")
	}
}

// A read that failed is not a name that is free.
//
// Installing checks whether the name is taken. If any failure counted as "not
// taken", a refused Secret read or a lost connection would each present as a
// clean slate and the install would go ahead against a namespace nobody could
// see into — writing over whatever was already there.
func TestInstallRefusesWhenTheExistingReleaseCannotBeRead(t *testing.T) {
	tests := []struct {
		name    string
		readErr error
		wantErr error
	}{
		{name: "the panel may not read the namespace", readErr: ErrForbidden, wantErr: ErrForbidden},
		{name: "the read failed for some other reason", readErr: errors.New("connection refused"),
			wantErr: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newWriteRepo()
			repo.readErr = test.readErr

			_, err := writeService(repo).Install(t.Context(),
				InstallRequest{Namespace: "apps", Name: "whoami", Chart: "whoami", Version: "1.0.0"})

			if err == nil {
				t.Fatal("the install was accepted despite an unreadable namespace")
			}
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			repo.mu.Lock()
			defer repo.mu.Unlock()
			if len(repo.installed) != 0 {
				t.Fatal("a refused install still ran")
			}
		})
	}
}

// The pipeline path, and the one that must not regress: a body carrying only a
// version keeps the release's own values.
func TestUpgradeWithoutValuesReusesTheReleasesOwn(t *testing.T) {
	repo := newWriteRepo()
	repo.existing["whoami"] = ReleaseDetail{Release: Release{Name: "whoami", Chart: "whoami"}}

	if _, err := writeService(repo).Upgrade(t.Context(),
		UpgradeRequest{Namespace: "apps", Name: "whoami", Version: "1.1.0"}); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	repo.await(t)

	if got := repo.upgraded[0].Values; got != nil {
		t.Errorf("values = %v, want nil so the release keeps its own", got)
	}
	if repo.upgraded[0].Version != "1.1.0" {
		t.Errorf("version = %q, want the requested one", repo.upgraded[0].Version)
	}
}

// ...and values that were sent are passed through untouched.
func TestUpgradeWithValuesPassesThem(t *testing.T) {
	repo := newWriteRepo()
	repo.existing["whoami"] = ReleaseDetail{Release: Release{Name: "whoami", Chart: "whoami"}}

	values := map[string]any{"replicas": float64(3)}
	if _, err := writeService(repo).Upgrade(t.Context(),
		UpgradeRequest{Namespace: "apps", Name: "whoami", Version: "1.1.0", Values: values}); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	repo.await(t)

	if got := repo.upgraded[0].Values["replicas"]; got != float64(3) {
		t.Errorf("values = %v, want them passed through", repo.upgraded[0].Values)
	}
}

// The panel can see everything Helm put in a managed namespace and can only
// change what was vetted.
func TestUpgradeRefusesAReleaseFromOutsideTheCatalog(t *testing.T) {
	repo := newWriteRepo()
	repo.existing["legacy"] = ReleaseDetail{Release: Release{Name: "legacy", Chart: "something-else"}}

	_, err := writeService(repo).Upgrade(t.Context(),
		UpgradeRequest{Namespace: "apps", Name: "legacy", Version: "1.0.0"})
	if !errors.Is(err, ErrUnknownChart) {
		t.Fatalf("error = %v, want %v", err, ErrUnknownChart)
	}
	if len(repo.upgraded) != 0 {
		t.Fatal("a refused upgrade still ran")
	}
}

// Upgrading something that is not there is a 404, not an install.
func TestUpgradeRefusesAReleaseThatIsNotThere(t *testing.T) {
	repo := newWriteRepo()
	_, err := writeService(repo).Upgrade(t.Context(),
		UpgradeRequest{Namespace: "apps", Name: "whoami", Version: "1.0.0"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want %v", err, ErrNotFound)
	}
}

func TestRollback(t *testing.T) {
	tests := []struct {
		name     string
		revision int
		wantErr  error
	}{
		{name: "a revision the release has", revision: 2},
		{name: "revision zero, which Helm would read as the previous one", revision: 0, wantErr: ErrInvalidName},
		{name: "a negative revision", revision: -1, wantErr: ErrInvalidName},
		{name: "a revision beyond the history", revision: 99, wantErr: ErrNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newWriteRepo()
			repo.existing["whoami"] = ReleaseDetail{Release: Release{Name: "whoami", Chart: "whoami"}}
			repo.history = []Revision{{Revision: 3}, {Revision: 2}, {Revision: 1}}

			_, err := writeService(repo).Rollback(t.Context(), "apps", "whoami", test.revision)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr != nil {
				if len(repo.rolled) != 0 {
					t.Fatal("a refused rollback still ran")
				}
				return
			}
			repo.await(t)
			if repo.rolled[0] != test.revision {
				t.Errorf("rolled back to %d, want %d", repo.rolled[0], test.revision)
			}
		})
	}
}

// Two operations on one release do not start at once. This is not the real guard
// -- two replicas can still race, and Helm's own storage is what stops that --
// but it turns a double-clicked button into a clean refusal.
func TestASecondOperationOnTheSameReleaseIsRefused(t *testing.T) {
	repo := newWriteRepo()
	repo.existing["whoami"] = ReleaseDetail{Release: Release{Name: "whoami", Chart: "whoami"}}
	repo.blocked = make(chan struct{})
	service := writeService(repo)

	if _, err := service.Upgrade(t.Context(),
		UpgradeRequest{Namespace: "apps", Name: "whoami", Version: "1.0.0"}); err != nil {
		t.Fatalf("the first upgrade was refused: %v", err)
	}

	_, err := service.Upgrade(t.Context(),
		UpgradeRequest{Namespace: "apps", Name: "whoami", Version: "1.1.0"})
	if !errors.Is(err, ErrInProgress) {
		t.Fatalf("error = %v, want %v", err, ErrInProgress)
	}

	close(repo.blocked)
	repo.await(t)

	if len(repo.upgraded) != 1 {
		t.Errorf("the storage ran %d upgrades, want only the first", len(repo.upgraded))
	}
}

// ...and a different release is not blocked by it. The lock is per release, or
// one slow install would stop every other deploy in the lab.
func TestADifferentReleaseIsNotBlocked(t *testing.T) {
	repo := newWriteRepo()
	repo.existing["one"] = ReleaseDetail{Release: Release{Name: "one", Chart: "whoami"}}
	repo.existing["two"] = ReleaseDetail{Release: Release{Name: "two", Chart: "whoami"}}
	repo.blocked = make(chan struct{})
	service := writeService(repo)

	for _, name := range []string{"one", "two"} {
		if _, err := service.Upgrade(t.Context(),
			UpgradeRequest{Namespace: "apps", Name: name, Version: "1.0.0"}); err != nil {
			t.Fatalf("upgrading %s was refused: %v", name, err)
		}
	}

	close(repo.blocked)
	repo.await(t)
	repo.await(t)
}

// The lock is released whatever happened, or one failed deploy would refuse
// every later attempt at that release until the pod restarted.
//
// The second attempt retries rather than being issued once. The fake signals from
// inside the operation, and the lock is released by a defer that runs after it —
// so "the storage was reached" and "the lock is free again" are two moments, in
// that order, and asserting the second by observing the first is a race that
// passes almost always. Retrying until a deadline asserts the thing the test is
// named for: that the lock becomes available, not that it already has.
func TestTheLockIsReleasedAfterAnOperationFinishes(t *testing.T) {
	repo := newWriteRepo()
	repo.existing["whoami"] = ReleaseDetail{Release: Release{Name: "whoami", Chart: "whoami"}}
	service := writeService(repo)

	upgrade := func() error {
		_, err := service.Upgrade(t.Context(),
			UpgradeRequest{Namespace: "apps", Name: "whoami", Version: "1.0.0"})
		return err
	}

	if err := upgrade(); err != nil {
		t.Fatalf("the first upgrade was refused: %v", err)
	}
	repo.await(t)

	deadline := time.Now().Add(2 * time.Second)
	var err error
	for time.Now().Before(deadline) {
		if err = upgrade(); err == nil {
			repo.await(t)
			return
		}
		if !errors.Is(err, ErrInProgress) {
			t.Fatalf("the second upgrade failed for the wrong reason: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("the lock was never released: %v", err)
}

func TestUninstall(t *testing.T) {
	repo := newWriteRepo()
	repo.existing["whoami"] = ReleaseDetail{Release: Release{Name: "whoami", Chart: "whoami"}}

	accepted, err := writeService(repo).Uninstall(t.Context(), "apps", "whoami")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if accepted.Operation != "uninstall" {
		t.Errorf("operation = %q, want uninstall", accepted.Operation)
	}
	repo.await(t)
	if repo.removed[0] != "whoami" {
		t.Errorf("removed %q, want whoami", repo.removed[0])
	}

	// A protected namespace is refused for uninstall too. Every write has to
	// agree, or the one that forgets is the way in.
	if _, err := writeService(newWriteRepo()).
		Uninstall(t.Context(), "platform-system", "whoami"); !errors.Is(err, ErrProtected) {
		t.Errorf("uninstalling from a protected namespace = %v, want %v", err, ErrProtected)
	}
}

// Values end up in a Secret, which etcd caps. Refusing here means the refusal
// lands before the chart is applied rather than after.
func TestValuesTooLargeAreRefused(t *testing.T) {
	repo := newWriteRepo()
	big := make([]byte, maxValuesBytes+1)
	for i := range big {
		big[i] = 'a'
	}

	_, err := writeService(repo).Install(t.Context(), InstallRequest{
		Namespace: "apps", Name: "whoami", Chart: "whoami", Version: "1.0.0",
		Values: map[string]any{"data": string(big)},
	})
	if !errors.Is(err, ErrValuesTooLarge) {
		t.Fatalf("error = %v, want %v", err, ErrValuesTooLarge)
	}
	if len(repo.installed) != 0 {
		t.Fatal("a refused install still ran")
	}
}

// An unreachable repository degrades the catalog listing and refuses an install.
// Installing something nothing could confirm is worse than not installing it.
func TestInstallRefusesWhenTheRepositoryCannotBeReached(t *testing.T) {
	repo := newWriteRepo()
	repo.versionErr = ErrRepositoryUnreachable

	_, err := writeService(repo).Install(t.Context(),
		InstallRequest{Namespace: "apps", Name: "whoami", Chart: "whoami", Version: "1.0.0"})
	if !errors.Is(err, ErrRepositoryUnreachable) {
		t.Fatalf("error = %v, want %v", err, ErrRepositoryUnreachable)
	}
	if len(repo.installed) != 0 {
		t.Fatal("a refused install still ran")
	}
}
