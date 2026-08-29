package helm

import (
	"context"
	"fmt"
	"io"
	"sort"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	apiwatch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	typedbatchv1 "k8s.io/client-go/kubernetes/typed/batch/v1"
	"k8s.io/client-go/tools/watch"

	"github.com/eetr-ai/home-lab/admin/api/internal/restconfig"
)

// JobRepository creates and reads the Jobs that perform Helm operations.
//
// Separate from Repository because the two speak different languages: that one
// talks to Helm, this one talks to Kubernetes, and keeping repo.go the only
// importer of helm.sh/helm is what stops a Helm minor upgrade reaching the wire
// format.
//
// It builds its own clientsets from restconfig rather than borrowing the
// Kubernetes slice's. That is the reuse restconfig exists for — a slice never
// imports another slice's internals, and NewStreamClientset lives behind that
// seam.
type JobRepository struct {
	// clientset bounds every request at twenty seconds, which is right for a
	// create and a get.
	clientset kubernetes.Interface
	// streamClient has no deadline. A watch on a Job outlasts a deploy and a log
	// follow outlasts both, and a twenty-second ceiling would cut each of them
	// off mid-way.
	streamClient kubernetes.Interface
	config       JobConfig
}

// NewJobRepository builds the repository. It contacts nothing.
func NewJobRepository(config JobConfig) (*JobRepository, error) {
	bounded, err := restconfig.New()
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(bounded)
	if err != nil {
		return nil, fmt.Errorf("build the Kubernetes client for Helm jobs: %w", err)
	}

	unbounded, err := restconfig.NewUnbounded()
	if err != nil {
		return nil, err
	}
	streamClient, err := kubernetes.NewForConfig(unbounded)
	if err != nil {
		return nil, fmt.Errorf("build the streaming Kubernetes client for Helm jobs: %w", err)
	}

	return &JobRepository{clientset: clientset, streamClient: streamClient, config: config}, nil
}

// CreateJob starts one Helm operation and returns the Job the API server named.
func (r *JobRepository) CreateJob(ctx context.Context, spec JobSpec, ref ReleaseRef,
	actor string,
) (Job, error) {
	created, err := r.clientset.BatchV1().Jobs(r.config.Namespace).
		Create(ctx, buildJob(spec, ref, actor, r.config), metav1.CreateOptions{})
	if err != nil {
		return Job{}, translate(err, "start a job for "+ref.Release)
	}
	return jobFrom(created, ""), nil
}

// ReadJob returns one Job, with the pod running it when there is one.
func (r *JobRepository) ReadJob(ctx context.Context, name string) (Job, error) {
	found, err := r.clientset.BatchV1().Jobs(r.config.Namespace).
		Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return Job{}, translate(err, "read job "+name)
	}
	pod, _ := r.podFor(ctx, found)
	return jobFrom(found, pod), nil
}

// ListJobs returns the Helm jobs matching a filter, newest first.
func (r *JobRepository) ListJobs(ctx context.Context, filter JobFilter) ([]Job, error) {
	found, err := r.clientset.BatchV1().Jobs(r.config.Namespace).
		List(ctx, metav1.ListOptions{LabelSelector: filter.selector()})
	if err != nil {
		return nil, translate(err, "list helm jobs")
	}

	jobs := make([]Job, 0, len(found.Items))
	for i := range found.Items {
		jobs = append(jobs, jobFrom(&found.Items[i], ""))
	}
	// Newest first: the one an operator wants is almost always the last one
	// started, and a page showing a finished job above a running one would be
	// answering a question nobody asked.
	sort.Slice(jobs, func(a, b int) bool { return jobs[a].CreatedAt.After(jobs[b].CreatedAt) })
	return jobs, nil
}

// WatchJob reports a Job's status as it changes, until the context ends or the
// Job reaches a terminal phase.
//
// The channel is closed when there is nothing more to report. A watch that the
// API server closes on its own — which it does periodically, and always
// eventually — is re-established underneath rather than surfaced: RetryWatcher
// resumes from the last resourceVersion it saw, which is what makes this
// survivable without any state of our own.
func (r *JobRepository) WatchJob(ctx context.Context, name string) (<-chan Job, error) {
	jobs := r.streamClient.BatchV1().Jobs(r.config.Namespace)

	initial, err := jobs.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, translate(err, "watch job "+name)
	}

	watcher, err := watch.NewRetryWatcherWithContext(ctx, initial.ResourceVersion,
		&jobListWatch{jobs: jobs, name: name})
	if err != nil {
		return nil, fmt.Errorf("watch job %s: %w", name, err)
	}

	updates := make(chan Job)
	go func() {
		defer close(updates)
		defer watcher.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case event, open := <-watcher.ResultChan():
				if !open {
					return
				}
				job, ok := event.Object.(*batchv1.Job)
				if !ok {
					continue
				}
				pod, _ := r.podFor(ctx, job)
				select {
				case updates <- jobFrom(job, pod):
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return updates, nil
}

// jobListWatch watches one Job by name.
//
// A field selector rather than a label selector: the name is exact, and this must
// keep answering for a Job whose labels somebody edited by hand.
type jobListWatch struct {
	jobs typedbatchv1.JobInterface
	name string
}

// The signature is fixed by cache.WatcherWithContext, which is what
// RetryWatcherWithContext consumes. Taking the context here rather than reaching
// for context.Background is the reason to use the context-aware watcher at all:
// a re-established watch is bounded by the caller's stream like the first one.
//
//nolint:ireturn // the signature is fixed by cache.WatcherWithContext
func (w *jobListWatch) WatchWithContext(ctx context.Context,
	options metav1.ListOptions,
) (apiwatch.Interface, error) {
	options.FieldSelector = fields.OneTermEqualSelector("metadata.name", w.name).String()

	watcher, err := w.jobs.Watch(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("watch job %s: %w", w.name, err)
	}
	return watcher, nil
}

// PodLogs opens the log of the pod running a Job.
//
// Through the unbounded client, because a follow stream legitimately outlasts any
// request deadline.
func (r *JobRepository) PodLogs(ctx context.Context, pod string, follow bool,
	tail int64,
) (io.ReadCloser, error) {
	options := &corev1.PodLogOptions{Follow: follow}
	if tail > 0 {
		options.TailLines = &tail
	}

	stream, err := r.streamClient.CoreV1().Pods(r.config.Namespace).
		GetLogs(pod, options).Stream(ctx)
	if err != nil {
		return nil, translate(err, "read the log of "+pod)
	}
	return stream, nil
}

// podFor finds the pod a Job created.
//
// By ownerReference rather than by a label, deliberately. The label Kubernetes
// puts on a Job's pods was spelled `job-name` and is now
// `batch.kubernetes.io/job-name`; both exist on some versions and only one on
// others, and selecting on either makes this depend on the cluster's version for
// no benefit. An ownerReference naming the Job's UID is exact and has not moved.
//
// A failure here is not an error for the caller: the pod may simply not exist
// yet, which is the normal state for the first moment of every operation.
func (r *JobRepository) podFor(ctx context.Context, job *batchv1.Job) (string, error) {
	pods, err := r.clientset.CoreV1().Pods(r.config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set{labelName: jobComponent}.String(),
	})
	if err != nil {
		return "", fmt.Errorf("find the pod for job %s: %w", job.Name, err)
	}

	for i := range pods.Items {
		for _, owner := range pods.Items[i].OwnerReferences {
			if owner.UID == job.UID {
				return pods.Items[i].Name, nil
			}
		}
	}
	return "", nil
}

// JobFilter narrows a listing to the jobs somebody is asking about.
type JobFilter struct {
	Namespace    string
	Release      string
	DeploymentID string
}

func (f JobFilter) selector() string {
	set := labels.Set{labelName: jobComponent}
	if f.Namespace != "" {
		set[labelNamespace] = f.Namespace
	}
	if f.Release != "" {
		set[labelRelease] = f.Release
	}
	if f.DeploymentID != "" {
		set[labelDeployment] = f.DeploymentID
	}
	return set.String()
}

// jobFrom reads a Kubernetes Job into this package's own type.
func jobFrom(job *batchv1.Job, pod string) Job {
	phase, reason := phaseOf(job.Status)

	converted := Job{
		Name:         job.Name,
		Namespace:    job.Labels[labelNamespace],
		Release:      job.Labels[labelRelease],
		Operation:    job.Labels[labelOperation],
		DeploymentID: job.Labels[labelDeployment],
		Phase:        phase,
		Reason:       reason,
		Actor:        job.Annotations[annotationActor],
		Pod:          pod,
		CreatedAt:    job.CreationTimestamp.Time,
	}
	if job.Status.StartTime != nil {
		converted.StartedAt = &job.Status.StartTime.Time
	}
	if job.Status.CompletionTime != nil {
		converted.FinishedAt = &job.Status.CompletionTime.Time
	}
	return converted
}
