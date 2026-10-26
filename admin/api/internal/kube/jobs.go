package kube

import (
	"context"
	"fmt"
	"sort"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CronJob is a schedule that creates Jobs.
type CronJob struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// Schedule is the cron line, as written.
	Schedule string `json:"schedule"`
	// TimeZone is empty when the controller's own zone applies.
	TimeZone  string `json:"timeZone,omitempty"`
	Suspended bool   `json:"suspended"`
	// Active counts the Jobs it has running now.
	Active int `json:"active"`
	// LastScheduled and LastSucceeded are the controller's account. A gap between
	// them is the first sign a schedule is failing quietly.
	LastScheduled *time.Time `json:"lastScheduled,omitempty"`
	LastSucceeded *time.Time `json:"lastSucceeded,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
}

// Job is one run of something to completion.
type Job struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// CronJob names the schedule that made it, when one did. A Job started by
	// hand from a schedule (kubectl create job --from) names it too.
	CronJob string `json:"cronJob,omitempty"`
	// Status is one word: pending, running, succeeded, or failed.
	Status string `json:"status"`
	// Reason says why, when a Job failed for a reason Kubernetes named.
	Reason string `json:"reason,omitempty"`
	// Succeeded and Completions read as "1/1"; Completions is 1 when unset.
	Succeeded   int32      `json:"succeeded"`
	Completions int32      `json:"completions"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// JobListing is a namespace's, or the cluster's, schedules and runs together,
// because a Job is most often read beside the CronJob that made it.
type JobListing struct {
	CronJobs []CronJob `json:"cronJobs"`
	Jobs     []Job     `json:"jobs"`
}

// The words a Job's status is reported in.
const (
	JobPending   = "pending"
	JobRunning   = "running"
	JobSucceeded = "succeeded"
	JobFailed    = "failed"
)

// ListJobs returns the CronJobs and Jobs in one namespace, or in every namespace
// when given AllNamespaces.
func (r *Repository) ListJobs(ctx context.Context, namespace string) (JobListing, error) {
	cronJobs, err := r.client.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return JobListing{}, translate(err, "list cron jobs")
	}
	jobs, err := r.client.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return JobListing{}, translate(err, "list jobs")
	}

	ret := JobListing{
		CronJobs: make([]CronJob, 0, len(cronJobs.Items)),
		Jobs:     make([]Job, 0, len(jobs.Items)),
	}
	for i := range cronJobs.Items {
		ret.CronJobs = append(ret.CronJobs, summarizeCronJob(&cronJobs.Items[i]))
	}
	for i := range jobs.Items {
		ret.Jobs = append(ret.Jobs, summarizeJob(&jobs.Items[i]))
	}

	// Schedules by namespace and name, like every other listing. Runs newest
	// first within that, because the one an operator came for is the latest.
	sort.Slice(ret.CronJobs, func(a, b int) bool {
		return ret.CronJobs[a].Namespace+"/"+ret.CronJobs[a].Name < ret.CronJobs[b].Namespace+"/"+ret.CronJobs[b].Name
	})
	sort.Slice(ret.Jobs, func(a, b int) bool {
		if ret.Jobs[a].Namespace != ret.Jobs[b].Namespace {
			return ret.Jobs[a].Namespace < ret.Jobs[b].Namespace
		}
		// Ties on the second fall back to the name, so two runs created
		// together do not swap places between one refresh and the next.
		if !ret.Jobs[a].CreatedAt.Equal(ret.Jobs[b].CreatedAt) {
			return ret.Jobs[a].CreatedAt.After(ret.Jobs[b].CreatedAt)
		}
		return ret.Jobs[a].Name < ret.Jobs[b].Name
	})
	return ret, nil
}

func summarizeCronJob(cronJob *batchv1.CronJob) CronJob {
	ret := CronJob{
		Namespace: cronJob.Namespace,
		Name:      cronJob.Name,
		Schedule:  cronJob.Spec.Schedule,
		Active:    len(cronJob.Status.Active),
		CreatedAt: cronJob.CreationTimestamp.Time,
	}
	if cronJob.Spec.TimeZone != nil {
		ret.TimeZone = *cronJob.Spec.TimeZone
	}
	if cronJob.Spec.Suspend != nil {
		ret.Suspended = *cronJob.Spec.Suspend
	}
	if cronJob.Status.LastScheduleTime != nil {
		ret.LastScheduled = &cronJob.Status.LastScheduleTime.Time
	}
	if cronJob.Status.LastSuccessfulTime != nil {
		ret.LastSucceeded = &cronJob.Status.LastSuccessfulTime.Time
	}
	return ret
}

func summarizeJob(job *batchv1.Job) Job {
	status, reason := jobStatus(job.Status)
	ret := Job{
		Namespace:   job.Namespace,
		Name:        job.Name,
		Status:      status,
		Reason:      reason,
		Succeeded:   job.Status.Succeeded,
		Completions: 1,
		CreatedAt:   job.CreationTimestamp.Time,
	}
	if job.Spec.Completions != nil {
		ret.Completions = *job.Spec.Completions
	}
	for _, owner := range job.OwnerReferences {
		if owner.Kind == "CronJob" {
			ret.CronJob = owner.Name
		}
	}
	if job.Status.StartTime != nil {
		ret.StartedAt = &job.Status.StartTime.Time
	}
	if job.Status.CompletionTime != nil {
		ret.FinishedAt = &job.Status.CompletionTime.Time
	}
	return ret
}

// jobStatus reads a Job's status as one word, and why.
//
// The terminal conditions come first: a Job that has just failed can briefly
// report an active pod too, and "running" for something that has already lost is
// the wrong thing to show. A failed pod count alone is not failure — with retries
// left another attempt is coming — so only the condition says it.
func jobStatus(status batchv1.JobStatus) (string, string) {
	for _, condition := range status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		// FailureTarget and SuccessCriteriaMet are the verdict arriving before
		// the pods have finished terminating; Failed and Complete follow them.
		case batchv1.JobComplete, batchv1.JobSuccessCriteriaMet:
			return JobSucceeded, ""
		case batchv1.JobFailed, batchv1.JobFailureTarget:
			return JobFailed, condition.Reason
		}
	}
	if status.Active > 0 {
		return JobRunning, ""
	}
	return JobPending, ""
}

// ListJobs returns the CronJobs and Jobs in one namespace.
func (s *Service) ListJobs(ctx context.Context, namespace string) (JobListing, error) {
	if err := validateNamespace(namespace); err != nil {
		return JobListing{}, err
	}
	ret, err := s.repo.ListJobs(ctx, namespace)
	if err != nil {
		return JobListing{}, fmt.Errorf("list the jobs in %s: %w", namespace, err)
	}
	return ret, nil
}

// ListAllJobs returns every CronJob and Job on the cluster. The same read grant
// as one namespace's: it is a ClusterRole, like pods and workloads.
func (s *Service) ListAllJobs(ctx context.Context) (JobListing, error) {
	return s.repo.ListJobs(ctx, AllNamespaces)
}
