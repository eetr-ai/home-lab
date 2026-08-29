package helm

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The labels every Helm Job carries.
//
// Identity is the labels, not the name: the name is generated, so it is for
// humans and for addressing one Job that is already known. These are what the
// concurrency check lists by and what the panel filters by.
//
// The prefix is this lab's own, matching home-lab.example/gateway-access.
const (
	labelName       = "app.kubernetes.io/name"
	labelManagedBy  = "app.kubernetes.io/managed-by"
	labelNamespace  = "home-lab.example/helm-namespace"
	labelRelease    = "home-lab.example/helm-release"
	labelOperation  = "home-lab.example/helm-operation"
	labelDeployment = "home-lab.example/helm-deployment"

	// annotationActor records who asked. An annotation rather than a label
	// because an email address is not a valid label value.
	annotationActor = "home-lab.example/helm-actor"

	// jobComponent is what these Jobs are, and the value of labelName.
	jobComponent = "admin-helm-job"
)

// The phases a Job can be in, as the panel shows them.
const (
	PhasePending   = "pending"
	PhaseRunning   = "running"
	PhaseSucceeded = "succeeded"
	PhaseFailed    = "failed"
)

// ReleaseRef names the release an operation targets.
//
// Carried beside the JobSpec rather than inside it because a rollout is addressed
// by deployment and does not name a release — but every Job is still labelled with
// the release it affects, which is what makes "is something already deploying
// this?" a question the cluster can answer.
type ReleaseRef struct {
	Namespace string
	Release   string
}

// Job is one Helm operation, as the Kubernetes Job performing it.
//
// Nothing here is stored. Every field is read off the Job object or derived from
// its labels, which is the same rule Release follows and for the same reason: a
// second account of an operation would disagree with the cluster's the first time
// a pod was killed.
type Job struct {
	Name string `json:"name"`
	// Namespace and Release are what the operation targets, not where the Job
	// runs — the Job always runs in the panel's own namespace.
	Namespace    string `json:"namespace"`
	Release      string `json:"release"`
	Operation    string `json:"operation"`
	DeploymentID string `json:"deploymentId,omitempty"`
	// Phase is pending, running, succeeded, or failed.
	Phase string `json:"phase"`
	// Reason says why, when a Job failed for a reason Kubernetes named —
	// DeadlineExceeded, BackoffLimitExceeded. Helm's own account of a failure is
	// in the log, not here.
	Reason string `json:"reason,omitempty"`
	Actor  string `json:"actor,omitempty"`
	// Pod is empty until one has been scheduled, which is not immediate.
	Pod        string     `json:"pod,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

// Active reports whether this Job is still doing something.
func (j Job) Active() bool {
	return j.Phase == PhasePending || j.Phase == PhaseRunning
}

// JobConfig is everything about a Job's pod that does not come from the request.
//
// Which is all of it. The image, the ServiceAccount, the namespace and the
// command are read from this process's own environment; only the four validated
// scalars in JobSpec are influenced by a caller. That is deliberate and it is the
// security property it looks like — see the warning in buildJob.
type JobConfig struct {
	// Namespace is the panel's own, and the only namespace a Job is ever created
	// in. It is not the namespace the operation targets.
	Namespace       string
	Image           string
	ImagePullPolicy corev1.PullPolicy
	PullSecrets     []string
	ServiceAccount  string
	TTLSeconds      int32
	Timeout         time.Duration
	Resources       corev1.ResourceRequirements
	// DSNSecretName and DSNSecretKey locate the database credential. The name and
	// the key, never the value — see buildJob.
	DSNSecretName string
	DSNSecretKey  string
}

// helmCacheSize bounds the chart cache the Job unpacks into.
//
// The same figure the API pod's cache carries, and for the same reason: an
// unbounded scratch directory on a node is somebody else's outage.
var helmCacheSize = resource.MustParse("256Mi")

// deadlineSlack is how long the Job may run past Helm's own timeout.
//
// The runner bounds itself with the same timeout, so this should never fire. It
// catches a runner wedged somewhere that bound does not cover, and DeadlineExceeded
// is a legible terminal state where a pod running forever is not.
const deadlineSlack = 2 * time.Minute

// maxGeneratedNameBase leaves room for the suffix the API server appends.
//
// A generated name is the base plus five characters, and the whole must still be
// a 63-character DNS label. A 63-character namespace with a 53-character release
// would overflow the base on its own, so it is truncated rather than trusted.
const maxGeneratedNameBase = 57

// buildJob renders one Helm operation as a Kubernetes Job.
//
// READ THIS BEFORE CHANGING THE LABELS.
//
// The absence of three markers is load-bearing. Helm adopts an object carrying
// app.kubernetes.io/managed-by: Helm together with the meta.helm.sh/release-name
// and meta.helm.sh/release-namespace annotations — and it decides what to delete
// on upgrade by diffing rendered manifests, which an imperatively created object
// appears in neither side of. Both facts together are why upgrading the panel's
// own chart does not touch the Job performing the upgrade, which is the entire
// mechanism by which a self-upgrade stopped being a special case. managed-by here
// is admin-api, not Helm, and there are no meta.helm.sh annotations. A tidy-up
// that "makes the labels consistent with the chart" would silently restore the
// old failure, in which the pod doing the upgrade is destroyed mid-wait and the
// release wedges in pending-upgrade forever.
//
// There are no ownerReferences either. Nothing owns these; ttlSecondsAfterFinished
// is the reaper.
//
// And nothing in this pod spec comes off the wire. The ServiceAccount, the image,
// the namespace and the command are all read from this process's configuration.
// That matters because the account this Job runs as holds the whole deploy grant:
// a request that could name one would be a request that could name any account in
// the namespace.
func buildJob(spec JobSpec, ref ReleaseRef, actor string, config JobConfig) *batchv1.Job {
	labels := map[string]string{
		labelName: jobComponent,
		// Not "Helm". See above.
		labelManagedBy: "admin-api",
		labelNamespace: ref.Namespace,
		labelRelease:   ref.Release,
		labelOperation: spec.Operation,
	}
	if spec.DeploymentID != "" {
		labels[labelDeployment] = spec.DeploymentID
	}

	annotations := map[string]string{}
	if actor != "" {
		annotations[annotationActor] = actor
	}

	deadline := int64((config.Timeout + deadlineSlack).Seconds())
	// Never retried. A Helm operation that failed part-way has left the release in
	// a pending state, so a second attempt is guaranteed to fail differently and
	// more confusingly than the first — and the panel would be showing two pods'
	// logs for one intent. Whether to try again is a person's decision.
	backoffLimit := int32(0)

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: generatedNameFor(spec.Operation, ref),
			Namespace:    config.Namespace,
			Labels:       labels,
			Annotations:  annotations,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			ActiveDeadlineSeconds:   &deadline,
			TTLSecondsAfterFinished: &config.TTLSeconds,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec:       jobPodSpec(spec, config),
			},
		},
	}
}

func jobPodSpec(spec JobSpec, config JobConfig) corev1.PodSpec {
	nonRoot := true
	noEscalation := false
	readOnlyRoot := true
	user := int64(65532)

	pod := corev1.PodSpec{
		RestartPolicy:      corev1.RestartPolicyNever,
		ServiceAccountName: config.ServiceAccount,
		SecurityContext: &corev1.PodSecurityContext{
			RunAsNonRoot:   &nonRoot,
			SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
		Containers: []corev1.Container{{
			Name:            "helm",
			Image:           config.Image,
			ImagePullPolicy: config.ImagePullPolicy,
			Args:            spec.Args(),
			Env:             jobEnv(config),
			Resources:       config.Resources,
			// Helm writes while it works: it downloads a chart, unpacks it, and
			// keeps an OCI layer cache. The root filesystem is read-only, so it is
			// pointed at an emptyDir — the same arrangement the API pod has, and
			// for the same reason.
			VolumeMounts: []corev1.VolumeMount{{Name: "helm-cache", MountPath: "/helm"}},
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: &noEscalation,
				Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
				ReadOnlyRootFilesystem:   &readOnlyRoot,
				RunAsNonRoot:             &nonRoot,
				RunAsUser:                &user,
				RunAsGroup:               &user,
			},
		}},
		Volumes: []corev1.Volume{{
			Name: "helm-cache",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: &helmCacheSize},
			},
		}},
	}

	for _, secret := range config.PullSecrets {
		pod.ImagePullSecrets = append(pod.ImagePullSecrets,
			corev1.LocalObjectReference{Name: secret})
	}
	return pod
}

// jobEnv is the ambient configuration the runner reads with os.Getenv.
//
// The database credential travels as a SecretKeyRef and never as a value. The API
// holds its own DSN in its environment and must not copy it here: a Job is not a
// Secret, it is readable by anything that can list Jobs in this namespace, and a
// connection string rendered into one is a credential in plain text. The chart
// hands this process the Secret's name and key for exactly that reason.
func jobEnv(config JobConfig) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{Name: "ADMIN_HELM_TIMEOUT", Value: config.Timeout.String()},
		// Helm's caches, pointed at the emptyDir because the root filesystem is
		// read-only. XDG_CACHE_HOME is here too because Helm's registry client
		// reaches for it directly rather than going through HELM_CACHE_HOME.
		{Name: "HELM_CACHE_HOME", Value: "/helm/cache"},
		{Name: "HELM_CONFIG_HOME", Value: "/helm/config"},
		{Name: "HELM_DATA_HOME", Value: "/helm/data"},
		{Name: "XDG_CACHE_HOME", Value: "/helm/cache"},
	}

	if config.DSNSecretName != "" {
		env = append(env, corev1.EnvVar{
			Name: "ADMIN_HELM_DSN",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: config.DSNSecretName},
					Key:                  config.DSNSecretKey,
				},
			},
		})
	}
	return env
}

// generatedNameFor builds the base the API server appends a suffix to.
//
// For humans reading `kubectl get jobs`. Identity is the labels.
func generatedNameFor(operation string, ref ReleaseRef) string {
	base := fmt.Sprintf("helm-%s-%s-%s-", operation, ref.Namespace, ref.Release)
	if len(base) > maxGeneratedNameBase {
		// Truncate one short of the limit so the separator fits, and trim any
		// separators the cut exposed so the base cannot end in "--".
		base = strings.TrimRight(base[:maxGeneratedNameBase-1], "-") + "-"
	}
	return base
}

// phaseOf reads a Job's status as one word, and why.
//
// The order matters. A terminal condition is checked before the active count,
// because a Job that has just failed can briefly report both — and "running" for
// something that has already lost is the answer a pipeline would act on.
//
// A failed *count* is deliberately not enough on its own: with a backoff limit
// above zero it means an attempt failed and another is coming, which is not the
// Job failing. This lab sets the limit to zero, so the two coincide today; reading
// the condition rather than the count is what keeps that a property of the Job
// spec rather than a coincidence this function depends on.
func phaseOf(status batchv1.JobStatus) (phase, reason string) {
	for _, condition := range status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case batchv1.JobComplete:
			return PhaseSucceeded, ""
		case batchv1.JobFailed:
			return PhaseFailed, condition.Reason
		}
	}

	if status.Active > 0 {
		return PhaseRunning, ""
	}
	return PhasePending, ""
}

// activeJob returns the Job already operating on a release, if there is one.
//
// This is NOT a lock, and reading it as one would be a mistake. Two replicas can
// both list, both see nothing, and both create: the check and the create are not
// one operation, and Kubernetes offers no way to make them one without taking the
// Job's name as the lock — which would collide with the finished Job whose status
// and logs are the whole point of running one.
//
// What actually prevents a double operation is what always did: Helm refuses an
// operation against a release its own storage has left in a pending state.
//
// It exists for the common case — a double-clicked button, a pipeline that
// retried — where the second attempt should be a clean 409 naming the job already
// doing the work. It is still strictly better than the per-process map it
// replaced, which could not see the other replica at all and forgot everything
// when a pod restarted.
func activeJob(jobs []Job) *Job {
	for i := range jobs {
		if jobs[i].Active() {
			return &jobs[i]
		}
	}
	return nil
}

// ReadJob returns one Helm job.
//
// The name is validated before it reaches the API server: it is a path segment
// from a URL, and a generated Job name is a DNS label like any other.
func (s *Service) ReadJob(ctx context.Context, name string) (Job, error) {
	if s.jobs == nil {
		return Job{}, ErrNotConfigured
	}
	if err := validateNamespace(name); err != nil {
		return Job{}, fmt.Errorf("%w: %q is not a job name", ErrInvalidName, name)
	}
	return s.jobs.ReadJob(ctx, name)
}

// ListJobs returns the Helm jobs matching a filter, newest first.
//
// This is what lets a browser that never saw the 202 find the operation in
// progress — a different operator's page, a pipeline's deploy, or the panel
// loading again after its own pods were replaced mid-upgrade. Without it, the job
// name would exist only in the reply to whoever started it.
//
// Unbounded, deliberately. These are reaped by ttlSecondsAfterFinished within a
// day, and a lab that has run enough Helm operations in a day for this to need a
// page has a different problem.
func (s *Service) ListJobs(ctx context.Context, filter JobFilter) ([]Job, error) {
	if s.jobs == nil {
		return nil, ErrNotConfigured
	}
	if filter.Namespace != "" {
		if err := s.checkNamespace(filter.Namespace); err != nil {
			return nil, err
		}
	}
	return s.jobs.ListJobs(ctx, filter)
}

// JobLogs opens the log of the pod performing an operation.
//
// The pod is found through the Job rather than named by the caller, so this
// cannot be used to read the log of an arbitrary pod in the panel's namespace —
// which is where the panel's own pods live.
func (s *Service) JobLogs(ctx context.Context, name string, follow bool,
	tail int64,
) (io.ReadCloser, error) {
	job, err := s.ReadJob(ctx, name)
	if err != nil {
		return nil, err
	}
	if job.Pod == "" {
		return nil, fmt.Errorf("%w: %s has no pod yet", ErrNoPodYet, name)
	}
	return s.jobs.PodLogs(ctx, job.Pod, follow, tail)
}
