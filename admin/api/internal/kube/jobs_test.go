package kube

import (
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestJobStatusPutsTheVerdictBeforeTheActivePod(t *testing.T) {
	failed := batchv1.JobCondition{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Reason: "DeadlineExceeded"}
	tests := []struct {
		name   string
		status batchv1.JobStatus
		want   string
		reason string
	}{
		{"not started", batchv1.JobStatus{}, JobPending, ""},
		{"running", batchv1.JobStatus{Active: 1}, JobRunning, ""},
		{"done", batchv1.JobStatus{Conditions: []batchv1.JobCondition{
			{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}}, JobSucceeded, ""},
		// Failed with a pod still counted active: failed, and why.
		{"failed with a pod left", batchv1.JobStatus{Active: 1, Conditions: []batchv1.JobCondition{failed}},
			JobFailed, "DeadlineExceeded"},
		// A false condition is not a verdict.
		{"failed condition not true", batchv1.JobStatus{Active: 1, Conditions: []batchv1.JobCondition{
			{Type: batchv1.JobFailed, Status: corev1.ConditionFalse}}}, JobRunning, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, reason := jobStatus(test.status)
			if got != test.want || reason != test.reason {
				t.Errorf("got %s (%s), want %s (%s)", got, reason, test.want, test.reason)
			}
		})
	}
}

// A Job names the schedule that made it, and runs list newest first within a
// namespace — the latest is the one somebody came to look at.
func TestJobsListWithTheirScheduleNewestFirst(t *testing.T) {
	now := time.Now()
	older := job("admin", "backup-weekly-111", now.Add(-time.Hour))
	older.OwnerReferences = []metav1.OwnerReference{{Kind: "CronJob", Name: "backup-weekly"}}
	newer := job("admin", "backup-weekly-run-abc", now)
	newer.OwnerReferences = []metav1.OwnerReference{{Kind: "CronJob", Name: "backup-weekly"}}
	client := fake.NewClientset(older, newer, job("apps", "migrate", now), &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Namespace: "admin", Name: "backup-weekly"},
		Spec:       batchv1.CronJobSpec{Schedule: "0 3 * * 0"},
	})

	listing, err := NewRepository(client, client, nil, false).ListJobs(t.Context(), AllNamespaces)
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.CronJobs) != 1 || listing.CronJobs[0].Schedule != "0 3 * * 0" {
		t.Errorf("unexpected cron jobs %+v", listing.CronJobs)
	}
	got := make([]string, 0, len(listing.Jobs))
	for _, one := range listing.Jobs {
		got = append(got, one.Namespace+"/"+one.Name+"<"+one.CronJob)
	}
	want := []string{"admin/backup-weekly-run-abc<backup-weekly", "admin/backup-weekly-111<backup-weekly", "apps/migrate<"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d: got %s, want %s", i, got[i], want[i])
		}
	}
}

// An empty namespace is two empty lists, never null — the panel walks both.
func TestAnEmptyNamespaceListsNoJobsAsEmptyLists(t *testing.T) {
	client := fake.NewClientset()
	listing, err := NewRepository(client, client, nil, false).ListJobs(t.Context(), "empty")
	if err != nil || listing.CronJobs == nil || listing.Jobs == nil {
		t.Errorf("expected two empty lists, got %+v, %v", listing, err)
	}
}

func job(namespace, name string, created time.Time) *batchv1.Job {
	return &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Namespace: namespace, Name: name, CreationTimestamp: metav1.NewTime(created),
	}}
}
