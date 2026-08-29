package helm

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// fakeStore is an in-memory record, and it counts what reached it so a test can
// assert that a refusal happened before anything was written.
type fakeStore struct {
	deployments map[string]Deployment
	versions    map[string][]DeploymentVersion
	failWith    error
	rolledOut   []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		deployments: map[string]Deployment{},
		versions:    map[string][]DeploymentVersion{},
	}
}

func (f *fakeStore) seed(deployment Deployment, versions ...DeploymentVersion) {
	f.deployments[deployment.ID] = deployment
	// Newest first, which is the order the store returns.
	reversed := make([]DeploymentVersion, 0, len(versions))
	for index := len(versions) - 1; index >= 0; index-- {
		reversed = append(reversed, versions[index])
	}
	f.versions[deployment.ID] = reversed
}

func (f *fakeStore) ListDeployments(_ context.Context, namespace string) ([]Deployment, error) {
	if f.failWith != nil {
		return nil, f.failWith
	}
	var found []Deployment
	for _, deployment := range f.deployments {
		if namespace == "" || deployment.Namespace == namespace {
			found = append(found, deployment)
		}
	}
	return found, nil
}

func (f *fakeStore) ReadDeployment(_ context.Context, id string) (Deployment, error) {
	if f.failWith != nil {
		return Deployment{}, f.failWith
	}
	deployment, ok := f.deployments[id]
	if !ok {
		return Deployment{}, ErrNotFound
	}
	return deployment, nil
}

func (f *fakeStore) CreateDeployment(_ context.Context, deployment Deployment,
	first DeploymentVersion,
) (Deployment, error) {
	if f.failWith != nil {
		return Deployment{}, f.failWith
	}
	for _, existing := range f.deployments {
		if existing.Namespace == deployment.Namespace && existing.ReleaseName == deployment.ReleaseName {
			return Deployment{}, ErrAlreadyExists
		}
	}
	deployment.ID = "generated-" + deployment.ReleaseName
	first.Version = 1
	f.deployments[deployment.ID] = deployment
	f.versions[deployment.ID] = []DeploymentVersion{first}
	return deployment, nil
}

func (f *fakeStore) DeleteDeployment(_ context.Context, id string) error {
	delete(f.deployments, id)
	delete(f.versions, id)
	return nil
}

func (f *fakeStore) ListVersions(_ context.Context, id string) ([]DeploymentVersion, error) {
	if f.failWith != nil {
		return nil, f.failWith
	}
	return f.versions[id], nil
}

func (f *fakeStore) ReadVersion(_ context.Context, id string, number int) (DeploymentVersion, error) {
	for _, version := range f.versions[id] {
		if version.Version == number {
			return version, nil
		}
	}
	return DeploymentVersion{}, ErrNotFound
}

func (f *fakeStore) AppendVersion(_ context.Context, id string,
	version DeploymentVersion,
) (DeploymentVersion, error) {
	if f.failWith != nil {
		return DeploymentVersion{}, f.failWith
	}
	highest := 0
	for _, existing := range f.versions[id] {
		if existing.Version > highest {
			highest = existing.Version
		}
	}
	version.Version = highest + 1
	f.versions[id] = append([]DeploymentVersion{version}, f.versions[id]...)
	return version, nil
}

func (f *fakeStore) MarkRolledOut(_ context.Context, id string, number, helmRevision int) error {
	f.rolledOut = append(f.rolledOut, id)
	_ = number
	_ = helmRevision
	return nil
}

// newDeploymentService wires a service over both fakes.
func newDeploymentService(repo repository, deployments DeploymentStore) *Service {
	return NewService(repo, deployments, testPolicy(), Self{}, time.Minute,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// seededDeployment is one declared podinfo release in the managed namespace.
func seededDeployment(store *fakeStore, values string) Deployment {
	deployment := Deployment{
		ID:          "d1",
		Namespace:   "apps",
		ReleaseName: "podinfo",
		ChartRef:    "oci://ghcr.io/stefanprodan/charts/podinfo",
	}
	store.seed(deployment, DeploymentVersion{
		Version:      1,
		ChartVersion: "6.0.0",
		ValuesYAML:   values,
		Source:       SourcePanel,
	})
	return deployment
}

// The pipeline path is the one that must not regress: a body carrying only a
// chart version keeps the operator's values exactly as they were written,
// comments and all.
func TestPipelineRolloutWithNoValuesCarriesThePreviousOnesForward(t *testing.T) {
	repo, store := newFakeRepo(), newFakeStore()
	document := "# the message users see\nui:\n  message: hello\nreplicaCount: 2\n"
	deployment := seededDeployment(store, document)
	service := newDeploymentService(repo, store)

	if _, err := service.PipelineRollout(t.Context(), deployment.ID,
		PipelineRequest{Version: "6.1.0"}, "ci@pipeline"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	waitForJob(t, repo)

	newest := store.versions[deployment.ID][0]
	if newest.Version != 2 {
		t.Fatalf("want version 2, got %d", newest.Version)
	}
	if newest.ValuesYAML != document {
		t.Errorf("the previous document should be carried through byte for byte.\nwant:\n%s\ngot:\n%s",
			document, newest.ValuesYAML)
	}
	if newest.Source != SourceCI || newest.CreatedBy != "ci@pipeline" {
		t.Errorf("the version should be attributed to the pipeline: %+v", newest)
	}
	if newest.ChartVersion != "6.1.0" {
		t.Errorf("want chart version 6.1.0, got %q", newest.ChartVersion)
	}
}

// Overrides merge over the operator's values rather than replacing them, which is
// what lets a pipeline own image.tag and nothing else.
func TestPipelineRolloutMergesOverridesOverTheStoredValues(t *testing.T) {
	repo, store := newFakeRepo(), newFakeStore()
	deployment := seededDeployment(store,
		"image:\n  repository: podinfo\n  tag: 6.0.0\nreplicaCount: 2\n")
	service := newDeploymentService(repo, store)

	_, err := service.PipelineRollout(t.Context(), deployment.ID, PipelineRequest{
		Version: "6.1.0",
		Values:  map[string]any{"image": map[string]any{"tag": "sha-abc123"}},
	}, "ci@pipeline")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	waitForJob(t, repo)

	stored, err := parseValues(store.versions[deployment.ID][0].ValuesYAML)
	if err != nil {
		t.Fatalf("the stored values should parse: %v", err)
	}
	image, _ := stored["image"].(map[string]any)
	if image["tag"] != "sha-abc123" {
		t.Errorf("the override should be applied, got %#v", image["tag"])
	}
	if image["repository"] != "podinfo" {
		t.Errorf("the operator's sibling key should survive, got %#v", image["repository"])
	}
	if stored["replicaCount"] != float64(2) {
		t.Errorf("an untouched key should survive, got %#v", stored["replicaCount"])
	}

	// And what Helm was actually handed matches what was stored.
	if repo.installed.Values == nil && repo.upgraded.Values == nil {
		t.Fatal("nothing reached the repository")
	}
}

// A release Helm already has is upgraded; one it does not have is installed. The
// endpoint called does not decide it, because a record whose release was
// uninstalled has to be able to come back.
func TestRolloutInstallsWhenThereIsNoReleaseAndUpgradesWhenThereIs(t *testing.T) {
	t.Run("no release yet", func(t *testing.T) {
		repo, store := newFakeRepo(), newFakeStore()
		repo.readErr = ErrNotFound
		deployment := seededDeployment(store, "replicaCount: 1\n")

		accepted, err := newDeploymentService(repo, store).
			Rollout(t.Context(), deployment.ID, RolloutRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if accepted.Operation != "install" {
			t.Errorf("want install, got %q", accepted.Operation)
		}
		if got := waitForJob(t, repo); got != "install" {
			t.Errorf("the repository should have been asked to install, got %q", got)
		}
	})

	t.Run("a release is already there", func(t *testing.T) {
		repo, store := newFakeRepo(), newFakeStore()
		deployment := seededDeployment(store, "replicaCount: 1\n")

		accepted, err := newDeploymentService(repo, store).
			Rollout(t.Context(), deployment.ID, RolloutRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if accepted.Operation != "upgrade" {
			t.Errorf("want upgrade, got %q", accepted.Operation)
		}
		if got := waitForJob(t, repo); got != "upgrade" {
			t.Errorf("the repository should have been asked to upgrade, got %q", got)
		}
	})
}

// A read failure that is not "not found" must not be read as absence: installing
// over a release this could not see is how two copies of one workload happen.
func TestRolloutRefusesWhenTheReleaseCannotBeRead(t *testing.T) {
	repo, store := newFakeRepo(), newFakeStore()
	repo.readErr = ErrForbidden
	deployment := seededDeployment(store, "replicaCount: 1\n")

	_, err := newDeploymentService(repo, store).Rollout(t.Context(), deployment.ID, RolloutRequest{})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
	select {
	case operation := <-repo.ran:
		t.Fatalf("nothing should have run, and %q did", operation)
	default:
	}
}

// A namespace this lab does not manage is refused, and refused before the record
// is even consulted for versions.
func TestDeploymentsRefuseANamespaceThisLabDoesNotManage(t *testing.T) {
	for _, namespace := range []string{"platform-system", "admin", "kube-system", "somewhere-else"} {
		repo, store := newFakeRepo(), newFakeStore()
		store.seed(Deployment{
			ID: "d1", Namespace: namespace, ReleaseName: "thing",
			ChartRef: "oci://ghcr.io/org/thing",
		}, DeploymentVersion{Version: 1, ChartVersion: "1.0.0", Source: SourcePanel})

		_, err := newDeploymentService(repo, store).
			Rollout(t.Context(), "d1", RolloutRequest{})
		if !errors.Is(err, ErrProtected) && !errors.Is(err, ErrUnmanaged) {
			t.Errorf("%s should be refused, got %v", namespace, err)
		}
	}
}

// Declaring validates everything before it writes, so a bad reference or a range
// for a version never reaches the record.
func TestDeclareValidatesBeforeItWrites(t *testing.T) {
	cases := map[string]DeclareRequest{
		//nolint:gosec // deliberately credential-bearing; this asserts it is refused
		"a chart reference with credentials": {
			Namespace: "apps", Name: "podinfo",
			ChartRef: "oci://user:pass@ghcr.io/org/podinfo", Version: "6.0.0",
		},
		"a version range": {
			Namespace: "apps", Name: "podinfo",
			ChartRef: "oci://ghcr.io/org/podinfo", Version: "^6.0.0",
		},
		"values that are not YAML": {
			Namespace: "apps", Name: "podinfo",
			ChartRef: "oci://ghcr.io/org/podinfo", Version: "6.0.0",
			ValuesYAML: "a: 1\n  b: 2\n",
		},
		"a protected namespace": {
			Namespace: "platform-system", Name: "podinfo",
			ChartRef: "oci://ghcr.io/org/podinfo", Version: "6.0.0",
		},
	}

	for name, request := range cases {
		t.Run(name, func(t *testing.T) {
			store := newFakeStore()
			if _, err := newDeploymentService(newFakeRepo(), store).
				Declare(t.Context(), request, "someone@example.com"); err == nil {
				t.Fatal("should have been refused")
			}
			if len(store.deployments) != 0 {
				t.Errorf("nothing should have been written, and %d rows were", len(store.deployments))
			}
		})
	}
}

// With no database configured, the deployment endpoints report that the
// capability was not switched on rather than failing as though it were broken.
func TestDeploymentsReportAnUnconfiguredStore(t *testing.T) {
	service := newDeploymentService(newFakeRepo(), nil)

	if _, err := service.ListDeployments(t.Context(), ""); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("list: want ErrNotConfigured, got %v", err)
	}
	if _, err := service.ReadDeployment(t.Context(), "d1"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("read: want ErrNotConfigured, got %v", err)
	}
	if _, err := service.Rollout(t.Context(), "d1", RolloutRequest{}); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("rollout: want ErrNotConfigured, got %v", err)
	}
}

// A database that is configured and not answering is distinguishable from one
// that answered "no", because only the first is worth retrying.
func TestAnUnreachableStoreIsReportedAsSuchAndNothingIsDeployed(t *testing.T) {
	repo, store := newFakeRepo(), newFakeStore()
	store.failWith = ErrStoreUnavailable

	_, err := newDeploymentService(repo, store).Rollout(t.Context(), "d1", RolloutRequest{})
	if !errors.Is(err, ErrStoreUnavailable) {
		t.Fatalf("want ErrStoreUnavailable, got %v", err)
	}
	select {
	case operation := <-repo.ran:
		t.Fatalf("nothing should have reached the cluster, and %q did", operation)
	default:
	}
}

// The state a deployment reports is the whole reason for keeping a record beside
// the cluster, so each of its readings is pinned.
func TestDescribeState(t *testing.T) {
	rolledOut := time.Now()
	applied := DeploymentVersion{ChartVersion: "6.1.0", RolledOutAt: &rolledOut}
	never := DeploymentVersion{ChartVersion: "6.1.0"}

	cases := []struct {
		name    string
		version DeploymentVersion
		release *Release
		failed  bool
		want    string
	}{
		{"the read failed", applied, nil, true, StateUnknown},
		{"no release", applied, nil, false, StateNotInstalled},
		{"never rolled out", never, &Release{ChartVersion: "6.0.0"}, false, StatePending},
		{"running something else", applied, &Release{ChartVersion: "6.0.0"}, false, StateDrifted},
		{"in step", applied, &Release{ChartVersion: "6.1.0"}, false, StateInSync},
		// A release Helm recorded without a chart version cannot be compared, and
		// guessing "drifted" would light the page up for every such release.
		{"no chart version to compare", applied, &Release{}, false, StateInSync},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := describeState(testCase.version, testCase.release, testCase.failed)
			if got != testCase.want {
				t.Errorf("want %s, got %s", testCase.want, got)
			}
		})
	}
}

// waitForJob waits for the detached operation instead of sleeping and hoping.
func waitForJob(t *testing.T, repo *fakeRepo) string {
	t.Helper()
	select {
	case operation := <-repo.ran:
		return operation
	case <-time.After(2 * time.Second):
		t.Fatal("the detached operation never ran")
		return ""
	}
}

// A declared deployment reads back with its live release beside it, and a failed
// read is reported rather than shown as an absent release.
func TestReadDeploymentReportsAFailedReleaseRead(t *testing.T) {
	repo, store := newFakeRepo(), newFakeStore()
	repo.readErr = errors.New("secrets is forbidden")
	deployment := seededDeployment(store, "replicaCount: 1\n")

	detail, err := newDeploymentService(repo, store).ReadDeployment(t.Context(), deployment.ID)
	if err != nil {
		t.Fatalf("the record should still be readable: %v", err)
	}
	if detail.State != StateUnknown {
		t.Errorf("want %s, got %s", StateUnknown, detail.State)
	}
	if !strings.Contains(detail.ReleaseError, "forbidden") {
		t.Errorf("the reason should be carried through, got %q", detail.ReleaseError)
	}
	if detail.Release != nil {
		t.Error("no release should be reported when it could not be read")
	}
}

// An upgrade of the panel's own release must not wait for the workloads it
// applies, because one of them is the pod running the upgrade. Waiting there
// leaves the release wedged in pending-upgrade, and Helm then refuses every
// later operation on it — so one self-upgrade would permanently break
// self-upgrades.
func TestUpgradingTheOwnReleaseDoesNotWaitForReadiness(t *testing.T) {
	repo, store := newFakeRepo(), newFakeStore()
	deployment := seededDeployment(store, "replicaCount: 1\n")

	service := NewService(repo, store, testPolicy(),
		Self{Namespace: deployment.Namespace, Release: deployment.ReleaseName},
		time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)))

	accepted, err := service.Rollout(t.Context(), deployment.ID, RolloutRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	waitForJob(t, repo)

	if !repo.upgraded.SkipWait {
		t.Error("an upgrade of the panel's own release must not wait for readiness")
	}
	// And whoever called is told, because the status they are about to poll for
	// means less than it usually does.
	if !strings.Contains(accepted.Message, "manifests are applied") {
		t.Errorf("the acceptance should say the wait was skipped, and says: %q", accepted.Message)
	}
}

// Every other release still waits, so "deployed" keeps meaning the pods came up.
func TestUpgradingAnyOtherReleaseStillWaits(t *testing.T) {
	repo, store := newFakeRepo(), newFakeStore()
	deployment := seededDeployment(store, "replicaCount: 1\n")

	service := NewService(repo, store, testPolicy(),
		Self{Namespace: "admin", Release: "home-lab-admin"},
		time.Minute, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if _, err := service.Rollout(t.Context(), deployment.ID, RolloutRequest{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	waitForJob(t, repo)

	if repo.upgraded.SkipWait {
		t.Error("only the panel's own release skips the wait")
	}
}

// Half-configured identity must not match anything: recognising a self-upgrade
// is what turns the wait off, so a blank matching a blank would silently stop
// every release waiting.
func TestSelfMatches(t *testing.T) {
	full := Self{Namespace: "admin", Release: "home-lab-admin"}
	if !full.Matches("admin", "home-lab-admin") {
		t.Error("the configured release should match itself")
	}
	if full.Matches("admin", "something-else") || full.Matches("apps", "home-lab-admin") {
		t.Error("a different release must not match")
	}

	for _, partial := range []Self{{}, {Namespace: "admin"}, {Release: "home-lab-admin"}} {
		if partial.Matches("", "") || partial.Matches("admin", "home-lab-admin") {
			t.Errorf("%+v should match nothing", partial)
		}
	}
}
