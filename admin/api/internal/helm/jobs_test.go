package helm

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// testJobConfig is the configuration the chart supplies at runtime.
//
// DSNSecretKey holds the NAME of a key inside a Secret, never a key. That no
// value ever reaches this process is the property
// TestBuildJobCarriesTheDSNAsAReferenceNotAValue exists to assert.
//
//nolint:gosec // a key name, not a key — see above
func testJobConfig() JobConfig {
	return JobConfig{
		Namespace:      "admin",
		Image:          "registry.example.invalid/home-lab/admin-api:1.2.3",
		ServiceAccount: "admin-helm-job",
		TTLSeconds:     86400,
		Timeout:        10 * time.Minute,
		DSNSecretName:  "admin-helm-postgres",
		DSNSecretKey:   "dsn",
	}
}

// The account a Helm Job runs as holds the whole deploy grant, and Kubernetes does
// not check whether the creator of a Job may run as it. So the only thing standing
// between "create a Job" and "name any ServiceAccount in this namespace" is that
// nothing in this pod spec comes off the wire. This is that assertion.
func TestBuildJobTakesNothingAboutThePodFromTheRequest(t *testing.T) {
	spec := JobSpec{Operation: OpRollout, DeploymentID: "d1", Version: 3}
	job := buildJob(spec, ReleaseRef{Namespace: "apps", Release: "podinfo"},
		"someone@example.com", testJobConfig())

	pod := job.Spec.Template.Spec
	if pod.ServiceAccountName != "admin-helm-job" {
		t.Errorf("serviceAccountName = %q, want the configured one", pod.ServiceAccountName)
	}
	if job.Namespace != "admin" {
		t.Errorf("namespace = %q, want the panel's own", job.Namespace)
	}
	if len(pod.Containers) != 1 {
		t.Fatalf("%d containers, want 1", len(pod.Containers))
	}
	if pod.Containers[0].Image != testJobConfig().Image {
		t.Errorf("image = %q, want the configured one", pod.Containers[0].Image)
	}
	// The args are the only caller-influenced part, and they are the four
	// validated scalars and nothing else.
	if got := pod.Containers[0].Args; got[0] != RunCommand || got[1] != OpRollout {
		t.Errorf("args = %v, want the helm-run subcommand", got)
	}
}

// A Job is not a Secret. Anything able to list Jobs in this namespace can read
// every value in one, so the database connection string must arrive as a
// reference the kubelet resolves and never as a literal the API copied out of its
// own environment.
func TestBuildJobCarriesTheDSNAsAReferenceNotAValue(t *testing.T) {
	job := buildJob(JobSpec{Operation: OpRollout, DeploymentID: "d1", Version: 1},
		ReleaseRef{Namespace: "apps", Release: "podinfo"}, "", testJobConfig())

	var dsn *corev1.EnvVar
	for i, env := range job.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "ADMIN_HELM_DSN" {
			dsn = &job.Spec.Template.Spec.Containers[0].Env[i]
		}
	}
	if dsn == nil {
		t.Fatal("a rollout job needs ADMIN_HELM_DSN to read the values to apply")
	}
	if dsn.Value != "" {
		t.Errorf("ADMIN_HELM_DSN carries a literal value: %q", dsn.Value)
	}
	if dsn.ValueFrom == nil || dsn.ValueFrom.SecretKeyRef == nil {
		t.Fatal("ADMIN_HELM_DSN must be a secretKeyRef")
	}
	if got := dsn.ValueFrom.SecretKeyRef.Name; got != "admin-helm-postgres" {
		t.Errorf("secret name = %q, want the configured one", got)
	}

	// And nothing anywhere in the object looks like a connection string. The
	// field check above passes on an object that also leaked it somewhere else.
	encoded, err := yaml.Marshal(job)
	if err != nil {
		t.Fatalf("marshal the job: %v", err)
	}
	for _, scheme := range []string{"postgres://", "postgresql://"} {
		if strings.Contains(string(encoded), scheme) {
			t.Errorf("the job object contains a %s connection string", scheme)
		}
	}
}

// READ THE COMMENT ON buildJob BEFORE CHANGING THIS.
//
// The absence of these three markers is what stops Helm adopting — and therefore
// deleting — the Job that is upgrading the panel's own chart. It is the entire
// mechanism by which a self-upgrade stopped being a special case, and it is one
// "make the labels consistent" edit away from being lost.
func TestBuildJobIsNotAdoptableByHelm(t *testing.T) {
	job := buildJob(JobSpec{Operation: OpRollout, DeploymentID: "d1", Version: 1},
		ReleaseRef{Namespace: "admin", Release: "home-lab-admin"}, "", testJobConfig())

	if got := job.Labels["app.kubernetes.io/managed-by"]; got == "Helm" {
		t.Error("managed-by: Helm makes this Job adoptable, and an upgrade would delete it mid-apply")
	}
	for _, annotation := range []string{"meta.helm.sh/release-name", "meta.helm.sh/release-namespace"} {
		if _, present := job.Annotations[annotation]; present {
			t.Errorf("%s makes this Job adoptable by a release", annotation)
		}
	}
	// Nothing owns these; the TTL is the reaper. An ownerReference would hand
	// them to the garbage collector instead, on something else's schedule.
	if len(job.OwnerReferences) != 0 {
		t.Errorf("ownerReferences = %v, want none", job.OwnerReferences)
	}
}

// A Helm operation that failed part-way has left the release pending, so a retry
// is guaranteed to fail differently and more confusingly than the first attempt —
// and the panel would be showing two pods' logs for one intent.
func TestBuildJobNeverRetries(t *testing.T) {
	job := buildJob(JobSpec{Operation: OpUninstall, Namespace: "apps", Release: "podinfo"},
		ReleaseRef{Namespace: "apps", Release: "podinfo"}, "", testJobConfig())

	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 0 {
		t.Errorf("backoffLimit = %v, want 0", job.Spec.BackoffLimit)
	}
	if got := job.Spec.Template.Spec.RestartPolicy; got != corev1.RestartPolicyNever {
		t.Errorf("restartPolicy = %q, want Never", got)
	}
	// The deadline has to outlast Helm's own timeout, or it fires first and every
	// slow-but-fine deploy is reported as DeadlineExceeded.
	if job.Spec.ActiveDeadlineSeconds == nil ||
		*job.Spec.ActiveDeadlineSeconds <= int64(testJobConfig().Timeout.Seconds()) {
		t.Errorf("activeDeadlineSeconds = %v, want more than the Helm timeout",
			job.Spec.ActiveDeadlineSeconds)
	}
}

// The API server appends five characters to a generated name and the result must
// still be a 63-character DNS label. The longest legal inputs overflow it, so the
// base is truncated rather than trusted.
func TestGeneratedNameLeavesRoomForTheSuffix(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		release   string
	}{
		{name: "ordinary", namespace: "apps", release: "podinfo"},
		{
			name:      "the longest names Kubernetes and Helm allow",
			namespace: strings.Repeat("n", maxNamespaceLength),
			release:   strings.Repeat("r", maxReleaseNameLength),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base := generatedNameFor(OpRollout, ReleaseRef{
				Namespace: test.namespace, Release: test.release,
			})
			if len(base) > maxGeneratedNameBase {
				t.Errorf("base is %d characters, want at most %d", len(base), maxGeneratedNameBase)
			}
			// A generated name is the base plus a suffix, so the base must end in
			// something a label may end in followed by the separator.
			if !strings.HasSuffix(base, "-") {
				t.Errorf("base %q should end with the separator", base)
			}
			if strings.HasSuffix(strings.TrimSuffix(base, "-"), "-") {
				t.Errorf("base %q leaves a double separator", base)
			}
		})
	}
}

// The labels are the identity — the name is generated, so nothing else can find
// a Job again. The concurrency check lists by these and the panel filters by them.
func TestBuildJobLabelsCarryTheTarget(t *testing.T) {
	job := buildJob(JobSpec{Operation: OpRollout, DeploymentID: "d1", Version: 1},
		ReleaseRef{Namespace: "apps", Release: "podinfo"}, "someone@example.com",
		testJobConfig())

	for label, want := range map[string]string{
		labelName:       jobComponent,
		labelNamespace:  "apps",
		labelRelease:    "podinfo",
		labelOperation:  OpRollout,
		labelDeployment: "d1",
	} {
		if got := job.Labels[label]; got != want {
			t.Errorf("%s = %q, want %q", label, got, want)
		}
	}
	// The pod carries them too, so a pod can be traced back without the Job.
	if job.Spec.Template.Labels[labelRelease] != "podinfo" {
		t.Error("the pod template should carry the same labels")
	}
	// An email is not a valid label value, so the actor is an annotation.
	if got := job.Annotations[annotationActor]; got != "someone@example.com" {
		t.Errorf("actor annotation = %q, want the caller", got)
	}
	if _, present := job.Labels[annotationActor]; present {
		t.Error("an email address is not a valid label value")
	}
}

// Reading a Job's status is the whole of "did it work", so the order these are
// asked in matters: a Job that has just failed can briefly report an active pod
// too, and "running" for something that has already lost is what a pipeline would
// act on.
func TestPhaseOf(t *testing.T) {
	condition := func(kind batchv1.JobConditionType, reason string) batchv1.JobCondition {
		return batchv1.JobCondition{
			Type: kind, Status: corev1.ConditionTrue, Reason: reason,
		}
	}

	tests := []struct {
		name       string
		status     batchv1.JobStatus
		wantPhase  string
		wantReason string
	}{
		{
			name:      "nothing has happened yet",
			status:    batchv1.JobStatus{},
			wantPhase: PhasePending,
		},
		{
			name:      "a pod is running",
			status:    batchv1.JobStatus{Active: 1},
			wantPhase: PhaseRunning,
		},
		{
			name: "it completed",
			status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{condition(batchv1.JobComplete, "")},
			},
			wantPhase: PhaseSucceeded,
		},
		{
			name: "it ran out of time",
			status: batchv1.JobStatus{
				Conditions: []batchv1.JobCondition{
					condition(batchv1.JobFailed, "DeadlineExceeded"),
				},
			},
			wantPhase:  PhaseFailed,
			wantReason: "DeadlineExceeded",
		},
		{
			// The ordering case. Both are true for a moment, and the terminal one
			// is the answer.
			name: "it failed while a pod is still counted active",
			status: batchv1.JobStatus{
				Active:     1,
				Conditions: []batchv1.JobCondition{condition(batchv1.JobFailed, "BackoffLimitExceeded")},
			},
			wantPhase:  PhaseFailed,
			wantReason: "BackoffLimitExceeded",
		},
		{
			// A condition that is present but false says nothing. Reading its type
			// without its status would report every Job as failed the moment
			// Kubernetes wrote a Failed=False condition onto it.
			name: "a condition that is not true",
			status: batchv1.JobStatus{
				Active: 1,
				Conditions: []batchv1.JobCondition{{
					Type: batchv1.JobFailed, Status: corev1.ConditionFalse,
				}},
			},
			wantPhase: PhaseRunning,
		},
		{
			// A failed *count* with no terminal condition means an attempt failed
			// and another may follow. This lab sets backoffLimit to 0 so the two
			// coincide today; reading the condition keeps that a property of the
			// Job spec rather than a coincidence this depends on.
			name:      "an attempt failed but the job has not",
			status:    batchv1.JobStatus{Failed: 1},
			wantPhase: PhasePending,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			phase, reason := phaseOf(test.status)
			if phase != test.wantPhase {
				t.Errorf("phase = %q, want %q", phase, test.wantPhase)
			}
			if reason != test.wantReason {
				t.Errorf("reason = %q, want %q", reason, test.wantReason)
			}
		})
	}
}

// A finished Job does not hold a release. Reading one as active would mean a
// release could be deployed exactly once until somebody deleted the Job.
func TestActiveJob(t *testing.T) {
	tests := []struct {
		name string
		jobs []Job
		want string
	}{
		{name: "nothing at all", jobs: nil},
		{
			name: "only finished ones",
			jobs: []Job{{Name: "a", Phase: PhaseSucceeded}, {Name: "b", Phase: PhaseFailed}},
		},
		{
			name: "one still running",
			jobs: []Job{{Name: "a", Phase: PhaseSucceeded}, {Name: "b", Phase: PhaseRunning}},
			want: "b",
		},
		{
			// Pending counts. A Job whose pod has not been scheduled yet is still
			// about to operate on the release.
			name: "one not yet scheduled",
			jobs: []Job{{Name: "a", Phase: PhasePending}},
			want: "a",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := activeJob(test.jobs)
			switch {
			case test.want == "" && got != nil:
				t.Errorf("found %q, want none", got.Name)
			case test.want != "" && got == nil:
				t.Errorf("found none, want %q", test.want)
			case test.want != "" && got.Name != test.want:
				t.Errorf("found %q, want %q", got.Name, test.want)
			}
		})
	}
}

// jobFrom reads identity out of the labels, so a Job missing them must come back
// empty rather than panicking — one created by hand, or by an older build, is a
// thing an operator can produce.
func TestJobFromToleratesMissingLabels(t *testing.T) {
	job := jobFrom(&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "stray"}}, "")

	if job.Name != "stray" {
		t.Errorf("name = %q, want stray", job.Name)
	}
	if job.Release != "" || job.Namespace != "" || job.Operation != "" {
		t.Errorf("want empty identity from an unlabelled job, got %+v", job)
	}
	if job.Phase != PhasePending {
		t.Errorf("phase = %q, want %q", job.Phase, PhasePending)
	}
}

// A job name arrives as a path segment from a URL, so it is validated before it
// reaches the API server rather than after.
func TestReadJobValidatesTheName(t *testing.T) {
	service := newDeploymentServiceWithJobs(newFakeRepo(), newFakeStore(), &fakeJobs{})

	for _, name := range []string{"", "Not-A-Label", "../secrets", strings.Repeat("a", 64)} {
		if _, err := service.ReadJob(t.Context(), name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("%q: want ErrInvalidName, got %v", name, err)
		}
	}
}

// The pod is found through the job, never named by the caller. Otherwise this
// would read the log of any pod in the panel's own namespace — which is where the
// panel's own pods live.
func TestJobLogsSaysSoWhenThereIsNoPodYet(t *testing.T) {
	runner := &fakeJobs{active: []Job{{Name: "helm-rollout-abcde", Phase: PhasePending}}}
	service := newDeploymentServiceWithJobs(newFakeRepo(), newFakeStore(), runner)

	_, err := service.JobLogs(t.Context(), "helm-rollout-abcde", false, 100)
	if !errors.Is(err, ErrNoPodYet) {
		t.Fatalf("want ErrNoPodYet, got %v", err)
	}
	// And not ErrNotFound, which would tell a client to stop rather than retry.
	if errors.Is(err, ErrNotFound) {
		t.Error("a pod that has not started yet must not read as a job that is gone")
	}
}

// A listing filtered to a namespace this lab does not manage is refused, like
// every other read of one.
func TestListJobsRefusesAnUnmanagedNamespace(t *testing.T) {
	service := newDeploymentServiceWithJobs(newFakeRepo(), newFakeStore(), &fakeJobs{})

	_, err := service.ListJobs(t.Context(), JobFilter{Namespace: "platform-system"})
	if !errors.Is(err, ErrProtected) {
		t.Fatalf("want ErrProtected, got %v", err)
	}
}

// The tail parameter is an optimization, so a malformed one must not fail the
// request — and an enormous one must not be honoured.
func TestTailLines(t *testing.T) {
	tests := map[string]int64{
		"":       defaultLogTail,
		"abc":    defaultLogTail,
		"0":      defaultLogTail,
		"-5":     defaultLogTail,
		"100":    100,
		"999999": maxLogTail,
	}
	for raw, want := range tests {
		if got := tailLines(raw); got != want {
			t.Errorf("tailLines(%q) = %d, want %d", raw, got, want)
		}
	}
}

// The chart passes resources as one JSON object so the values file keeps the
// normal Kubernetes shape. This asserts the two halves line up: what the chart
// renders, parsed the way main parses it, reaches the pod.
//
// Worth a test because the failure is silent in the wrong direction — a shape
// that did not unmarshal would leave the requests empty, and an empty
// ResourceRequirements is a BestEffort pod, which is the first thing evicted
// under node pressure. The symptom would be a release wedged half-applied, a
// long way from a values file.
func TestBuildJobCarriesTheResourcesTheChartRenders(t *testing.T) {
	// Verbatim from `helm template ... | grep ADMIN_HELM_JOB_RESOURCES`.
	const rendered = `{"limits":{"memory":"256Mi"},"requests":{"cpu":"25m","memory":"96Mi"}}`

	var resources corev1.ResourceRequirements
	if err := json.Unmarshal([]byte(rendered), &resources); err != nil {
		t.Fatalf("the chart's resources must parse as ResourceRequirements: %v", err)
	}

	config := testJobConfig()
	config.Resources = resources
	job := buildJob(JobSpec{Operation: OpRollout, DeploymentID: "d1", Version: 1},
		ReleaseRef{Namespace: "apps", Release: "podinfo"}, "", config)

	got := job.Spec.Template.Spec.Containers[0].Resources
	if got.Requests.Cpu().String() != "25m" || got.Requests.Memory().String() != "96Mi" {
		t.Errorf("requests = %v, want the ones the chart renders", got.Requests)
	}
	if got.Limits.Memory().String() != "256Mi" {
		t.Errorf("limits = %v, want the ones the chart renders", got.Limits)
	}
}

// A phase must never walk backwards.
//
// Found live: between the pod exiting and the controller writing Complete, a job
// reports no active pods and no terminal condition. Reading that as "pending"
// sends running → pending → succeeded, which a client watching transitions
// renders as the operation starting over.
func TestPhaseOfNeverRegressesWhileFinishing(t *testing.T) {
	started := metav1.Now()

	// Started, nothing active, no condition yet: the gap.
	phase, _ := phaseOf(batchv1.JobStatus{StartTime: &started})
	if phase != PhaseRunning {
		t.Errorf("phase = %q during the gap before Complete is written, want %q",
			phase, PhaseRunning)
	}

	// Genuinely not started yet is still pending — otherwise every job would
	// report running before it had a pod.
	phase, _ = phaseOf(batchv1.JobStatus{})
	if phase != PhasePending {
		t.Errorf("phase = %q before the job starts, want %q", phase, PhasePending)
	}
}
