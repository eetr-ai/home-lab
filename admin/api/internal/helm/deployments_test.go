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

// fakeJobs stands in for the cluster: it records what the service asked to run
// and can claim that something is already running.
type fakeJobs struct {
	created []JobSpec
	refs    []ReleaseRef
	// active is what ListJobs reports, so a test can put an operation in flight
	// without a cluster.
	active   []Job
	failWith error
}

func (f *fakeJobs) CreateJob(_ context.Context, spec JobSpec, ref ReleaseRef,
	_ string,
) (Job, error) {
	if f.failWith != nil {
		return Job{}, f.failWith
	}
	f.created = append(f.created, spec)
	f.refs = append(f.refs, ref)
	return Job{Name: "helm-" + spec.Operation + "-abcde", Operation: spec.Operation}, nil
}

func (f *fakeJobs) ListJobs(_ context.Context, _ JobFilter) ([]Job, error) {
	return f.active, nil
}

func (f *fakeJobs) ReadJob(_ context.Context, name string) (Job, error) {
	for _, job := range f.active {
		if job.Name == name {
			return job, nil
		}
	}
	return Job{}, ErrNotFound
}

func (f *fakeJobs) WatchJob(_ context.Context, _ string) (<-chan Job, error) {
	updates := make(chan Job)
	close(updates)
	return updates, nil
}

func (f *fakeJobs) PodLogs(_ context.Context, _ string, _ bool, _ int64) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

// newDeploymentService wires a service over the fakes.
func newDeploymentService(repo repository, deployments DeploymentStore) *Service {
	return newDeploymentServiceWithJobs(repo, deployments, &fakeJobs{})
}

func newDeploymentServiceWithJobs(repo repository, deployments DeploymentStore,
	runner Jobs,
) *Service {
	return NewService(repo, deployments, runner, testEnrolment(), testPolicy(), time.Minute,
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
	repo, store, runner := newFakeRepo(), newFakeStore(), &fakeJobs{}
	deployment := seededDeployment(store,
		"image:\n  repository: podinfo\n  tag: 6.0.0\nreplicaCount: 2\n")
	service := newDeploymentServiceWithJobs(repo, store, runner)

	_, err := service.PipelineRollout(t.Context(), deployment.ID, PipelineRequest{
		Version: "6.1.0",
		Values:  map[string]any{"image": map[string]any{"tag": "sha-abc123"}},
	}, "ci@pipeline")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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

	// And the version the Job was pointed at is the one that was just written.
	// That is the whole addressing scheme: the values never travel through the
	// Job, so a version appended between here and the pod starting cannot change
	// what gets applied.
	if len(runner.created) != 1 {
		t.Fatalf("%d jobs created, want 1", len(runner.created))
	}
	if got := runner.created[0].Version; got != 2 {
		t.Errorf("the job was pointed at version %d, want 2", got)
	}
}

// A rollout is dispatched as a rollout, whichever it turns out to be.
//
// The API used to probe Helm and answer "install" or "upgrade". It no longer
// does, and that is the point rather than a simplification: the Job decides, from
// what Helm has at the moment the work starts. Between this answering and the pod
// starting, a release can be uninstalled or appear, and an answer given here would
// be a second opinion about something the cluster is about to be asked again.
//
// Which way the Job actually goes is exercised against a real cluster by hand —
// there is no fake Helm storage here, and inventing one would test the fake.
func TestRolloutIsDispatchedWithoutDecidingInstallOrUpgrade(t *testing.T) {
	for _, present := range []bool{true, false} {
		repo, store, runner := newFakeRepo(), newFakeStore(), &fakeJobs{}
		if !present {
			repo.readErr = ErrNotFound
		}
		deployment := seededDeployment(store, "replicaCount: 1\n")

		accepted, err := newDeploymentServiceWithJobs(repo, store, runner).
			Rollout(t.Context(), deployment.ID, RolloutRequest{}, "tester")
		if err != nil {
			t.Fatalf("release present=%v: unexpected error: %v", present, err)
		}
		if accepted.Operation != OpRollout {
			t.Errorf("release present=%v: operation = %q, want %q",
				present, accepted.Operation, OpRollout)
		}
		if accepted.Job == "" {
			t.Errorf("release present=%v: the acceptance should name the job doing the work", present)
		}
		if len(runner.created) != 1 {
			t.Fatalf("release present=%v: %d jobs created, want 1", present, len(runner.created))
		}
		// Addressed by deployment and version, never by chart or values: the Job
		// reads those from the record itself.
		if got := runner.created[0]; got.DeploymentID != deployment.ID || got.Version != 1 {
			t.Errorf("release present=%v: job spec = %+v, want deployment %s version 1",
				present, got, deployment.ID)
		}
	}
}

// A second operation on a release something is already deploying is a clean 409
// naming the job that holds it, rather than whatever error Helm produces for a
// release mid-flight.
func TestRolloutRefusesWhenAJobIsAlreadyRunning(t *testing.T) {
	repo, store := newFakeRepo(), newFakeStore()
	runner := &fakeJobs{active: []Job{{
		Name: "helm-rollout-podinfo-x7k2q", Operation: OpRollout, Phase: PhaseRunning,
	}}}
	deployment := seededDeployment(store, "replicaCount: 1\n")

	_, err := newDeploymentServiceWithJobs(repo, store, runner).
		Rollout(t.Context(), deployment.ID, RolloutRequest{}, "tester")
	if !errors.Is(err, ErrInProgress) {
		t.Fatalf("want ErrInProgress, got %v", err)
	}
	if !strings.Contains(err.Error(), "helm-rollout-podinfo-x7k2q") {
		t.Errorf("the refusal should name the job already running, and says: %v", err)
	}
	if len(runner.created) != 0 {
		t.Error("nothing should have been created while another job holds the release")
	}
}

// A job that has finished does not hold the release. Reading it as one would mean
// a release could be deployed exactly once until somebody deleted the Job.
func TestRolloutProceedsWhenTheOnlyJobIsFinished(t *testing.T) {
	repo, store := newFakeRepo(), newFakeStore()
	runner := &fakeJobs{active: []Job{{Name: "helm-rollout-old", Phase: PhaseSucceeded}}}
	deployment := seededDeployment(store, "replicaCount: 1\n")

	if _, err := newDeploymentServiceWithJobs(repo, store, runner).
		Rollout(t.Context(), deployment.ID, RolloutRequest{}, "tester"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.created) != 1 {
		t.Errorf("%d jobs created, want 1", len(runner.created))
	}
}

// A read failure that is not "not found" must not be read as absence: installing
// over a release this could not see is how two copies of one workload happen.
func TestRolloutRefusesWhenTheReleaseCannotBeRead(t *testing.T) {
	repo, store := newFakeRepo(), newFakeStore()
	repo.readErr = ErrForbidden
	deployment := seededDeployment(store, "replicaCount: 1\n")

	_, err := newDeploymentService(repo, store).Rollout(t.Context(), deployment.ID, RolloutRequest{}, "tester")
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
			Rollout(t.Context(), "d1", RolloutRequest{}, "tester")
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
//
// And they name the RIGHT missing thing. This used to be ErrNotConfigured, which
// the handler renders as "no namespaces are configured for Helm" — so an
// operator whose namespaces were fine and whose database was absent was sent to
// look at the namespaces. The two conditions are independent: reading releases
// works without a store, and declaring one needs it however many namespaces are
// enrolled.
func TestDeploymentsReportAnUnconfiguredStore(t *testing.T) {
	service := newDeploymentService(newFakeRepo(), nil)

	if _, err := service.ListDeployments(t.Context(), ""); !errors.Is(err, ErrNoDeploymentStore) {
		t.Errorf("list: want ErrNoDeploymentStore, got %v", err)
	}
	if _, err := service.ReadDeployment(t.Context(), "d1"); !errors.Is(err, ErrNoDeploymentStore) {
		t.Errorf("read: want ErrNoDeploymentStore, got %v", err)
	}
	if _, err := service.Rollout(t.Context(), "d1", RolloutRequest{}, "tester"); !errors.Is(err, ErrNoDeploymentStore) {
		t.Errorf("rollout: want ErrNoDeploymentStore, got %v", err)
	}
	// The missing store must not masquerade as the other condition, which is what
	// this whole split is for.
	if _, err := service.ListDeployments(t.Context(), ""); errors.Is(err, ErrNotConfigured) {
		t.Error("a missing store must not report as an unconfigured namespace list")
	}
}

// A database that is configured and not answering is distinguishable from one
// that answered "no", because only the first is worth retrying.
func TestAnUnreachableStoreIsReportedAsSuchAndNothingIsDeployed(t *testing.T) {
	repo, store := newFakeRepo(), newFakeStore()
	store.failWith = ErrStoreUnavailable

	_, err := newDeploymentService(repo, store).Rollout(t.Context(), "d1", RolloutRequest{}, "tester")
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

// The panel's own release now waits like every other one.
//
// There used to be two tests here asserting the opposite: that an upgrade of this
// release skipped the readiness wait, and that every other release did not. That
// was never desirable — it made "deployed" mean "the manifests were accepted" for
// exactly the release an operator most wants a real answer about. It existed
// because the pod running the upgrade was one of the pods being replaced.
//
// A Job is not replaced by the chart it applies, so the exception is gone rather
// than configured, and there is nothing left here to assert that a fake could
// see. What replaces it is a live check, recorded on the pull request: upgrade
// the admin release from the panel and watch the Job outlive both Deployments
// rolling.

// The unscoped listing is checked too, and it is the one that had nothing
// checking it.
//
// A declared deployment outlives the enrolment that made its namespace
// deployable — revoking removes role bindings and leaves the record — so the
// store holds rows for namespaces this panel may no longer touch. Asked for one
// of them by name, every other route answers 403; the listing used to hand them
// over anyway, with the live release status read out of the namespace beside it.
func TestUnscopedDeploymentListingDropsNamespacesTheStackMayNotReach(t *testing.T) {
	store := newFakeStore()
	seededDeployment(store, "replicaCount: 1")
	store.seed(Deployment{
		ID:          "d2",
		Namespace:   "revoked",
		ReleaseName: "whoami",
		ChartRef:    "oci://ghcr.io/example/whoami",
	}, DeploymentVersion{Version: 1, ChartVersion: "1.0.0", Source: SourcePanel})

	summaries, err := newDeploymentService(newFakeRepo(), store).ListDeployments(t.Context(), "")
	if err != nil {
		t.Fatalf("ListDeployments() error = %v", err)
	}

	for _, summary := range summaries {
		if summary.Namespace == "revoked" {
			t.Errorf("the listing carried %s/%s, whose namespace is not enrolled",
				summary.Namespace, summary.ReleaseName)
		}
	}
	if len(summaries) != 1 {
		t.Errorf("listed %d deployments, want only the enrolled one", len(summaries))
	}
}
