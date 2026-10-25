package backup

import (
	"context"
	"fmt"
	"sort"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// componentSelector selects every object this slice made, and nothing a person
// named backup-something by hand.
var componentSelector = labels.Set{labelName: scheduleComponent, labelManagedBy: managedBy}.String()

// ListSchedules returns every schedule, by name.
func (r *Repository) ListSchedules(ctx context.Context) ([]Schedule, error) {
	found, err := r.clientset.BatchV1().CronJobs(r.namespace).
		List(ctx, metav1.ListOptions{LabelSelector: componentSelector})
	if err != nil {
		return nil, translate(err, "list the backup schedules")
	}

	ret := make([]Schedule, 0, len(found.Items))
	for i := range found.Items {
		ret = append(ret, scheduleFrom(&found.Items[i]))
	}
	sort.Slice(ret, func(a, b int) bool { return ret[a].Name < ret[b].Name })
	return ret, nil
}

// ReadSchedule returns one schedule.
func (r *Repository) ReadSchedule(ctx context.Context, name string) (Schedule, error) {
	cronJob, err := r.readCronJob(ctx, name)
	if err != nil {
		return Schedule{}, err
	}
	return scheduleFrom(cronJob), nil
}

// CreateSchedule creates the CronJob for a new schedule.
func (r *Repository) CreateSchedule(ctx context.Context, request ScheduleRequest, actor string) error {
	_, err := r.clientset.BatchV1().CronJobs(r.namespace).
		Create(ctx, buildCronJob(request, actor, r.schedules), metav1.CreateOptions{})
	return translate(err, "create the schedule "+request.Name)
}

// UpdateSchedule replaces a schedule's CronJob spec, keeping the object.
//
// Replaced from a fresh build rather than patched field by field, so a schedule
// created by an older panel picks up whatever this one renders — a new image,
// a changed deadline — the next time anybody saves it. The resourceVersion from
// the read is kept, so two saves racing each other make one of them a conflict
// rather than letting the second silently win.
func (r *Repository) UpdateSchedule(ctx context.Context, request ScheduleRequest, actor string) error {
	existing, err := r.readCronJob(ctx, request.Name)
	if err != nil {
		return err
	}

	updated := buildCronJob(request, actor, r.schedules)
	updated.ResourceVersion = existing.ResourceVersion
	_, err = r.clientset.BatchV1().CronJobs(r.namespace).Update(ctx, updated, metav1.UpdateOptions{})
	return translate(err, "update the schedule "+request.Name)
}

// DeleteSchedule deletes a schedule and, through ownership, its runs.
//
// The backups already in Drive stay where they are. Deleting a schedule is not a
// way to delete backups, and nothing here should make it one by accident.
func (r *Repository) DeleteSchedule(ctx context.Context, name string) error {
	if _, err := r.readCronJob(ctx, name); err != nil {
		return err
	}
	propagation := metav1.DeletePropagationBackground
	err := r.clientset.BatchV1().CronJobs(r.namespace).
		Delete(ctx, namePrefix+name, metav1.DeleteOptions{PropagationPolicy: &propagation})
	return translate(err, "delete the schedule "+name)
}

// RunSchedule starts a run of a schedule now.
//
// Refused while another run is going. The CronJob's Forbid policy only governs
// the runs the controller starts, so a manual one has to check for itself — and
// it matters, because two runs at once would both prune.
//
// The check and the create are two calls, so two presses landing together can
// still both start. That leaves one extra backup until the next run trims it,
// which is the cost of not having a lock, and is said here rather than hidden.
func (r *Repository) RunSchedule(ctx context.Context, name, actor string) (Run, error) {
	cronJob, err := r.readCronJob(ctx, name)
	if err != nil {
		return Run{}, err
	}
	runs, err := r.ListRuns(ctx, name)
	if err != nil {
		return Run{}, err
	}
	if active := activeRun(runs); active != "" {
		return Run{}, fmt.Errorf("%w: %s is still going", ErrInProgress, active)
	}
	created, err := r.clientset.BatchV1().Jobs(r.namespace).
		Create(ctx, manualJob(cronJob, actor), metav1.CreateOptions{})
	if err != nil {
		return Run{}, translate(err, "start a run of "+name)
	}
	return runFrom(created, ""), nil
}

// ListRuns returns a schedule's runs still on the cluster, newest first.
//
// The pods are found by ownerReference, the same way the Helm slice finds its
// Jobs' pods, and through the cluster-wide read grant in rbac.yaml — which is why
// rbac-backups.yaml has no pod rules of its own.
func (r *Repository) ListRuns(ctx context.Context, name string) ([]Run, error) {
	if _, err := r.readCronJob(ctx, name); err != nil {
		return nil, err
	}
	selector := labels.Set{labelName: scheduleComponent, labelSchedule: name}.String()

	jobs, err := r.clientset.BatchV1().Jobs(r.namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, translate(err, "list the runs of "+name)
	}
	pods, err := r.clientset.CoreV1().Pods(r.namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, translate(err, "list the pods of "+name)
	}

	podOf := map[string]string{}
	for i := range pods.Items {
		for _, owner := range pods.Items[i].OwnerReferences {
			podOf[string(owner.UID)] = pods.Items[i].Name
		}
	}

	ret := make([]Run, 0, len(jobs.Items))
	for i := range jobs.Items {
		ret = append(ret, runFrom(&jobs.Items[i], podOf[string(jobs.Items[i].UID)]))
	}
	sort.Slice(ret, func(a, b int) bool { return ret[a].CreatedAt.After(ret[b].CreatedAt) })
	return ret, nil
}

// readCronJob reads one schedule's CronJob, refusing one this slice did not make.
//
// A CronJob called backup-something that somebody applied by hand is not a
// schedule, and the panel editing or deleting it would be the panel reaching into
// something it was never given.
func (r *Repository) readCronJob(ctx context.Context, name string) (*batchv1.CronJob, error) {
	found, err := r.clientset.BatchV1().CronJobs(r.namespace).Get(ctx, namePrefix+name, metav1.GetOptions{})
	if err != nil {
		return nil, translate(err, "read the schedule "+name)
	}
	if found.Labels[labelName] != scheduleComponent || found.Labels[labelManagedBy] != managedBy {
		return nil, fmt.Errorf("%w: %s is not a schedule the panel made", ErrNotFound, name)
	}
	return found, nil
}

// activeRun names a run that has not finished, or returns empty.
func activeRun(runs []Run) string {
	for _, run := range runs {
		if run.Phase == PhasePending || run.Phase == PhaseRunning {
			return run.Name
		}
	}
	return ""
}
