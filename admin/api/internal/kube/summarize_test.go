package kube

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func containers(n int) []corev1.Container {
	out := make([]corev1.Container, n)
	for i := range out {
		out[i] = corev1.Container{Name: "c"}
	}
	return out
}

func waiting(reason string) corev1.ContainerStatus {
	return corev1.ContainerStatus{State: corev1.ContainerState{
		Waiting: &corev1.ContainerStateWaiting{Reason: reason}}}
}

func running(ready bool, restarts int32) corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Ready:        ready,
		RestartCount: restarts,
		State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
	}
}

// The phase alone is not the answer, which is the whole reason podStatus exists.
func TestSummarizePodStatus(t *testing.T) {
	deleted := metav1.NewTime(time.Now())

	tests := []struct {
		name         string
		pod          corev1.Pod
		wantStatus   string
		wantReady    string
		wantRestarts int32
	}{
		{
			name: "a healthy pod",
			pod: corev1.Pod{
				Spec:   corev1.PodSpec{Containers: containers(2)},
				Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{running(true, 0), running(true, 0)}},
			},
			wantStatus: "Running", wantReady: "2/2",
		},
		{
			// Running but not serving. The phase says Running, and the pod is
			// answering nothing — this is the case the summary exists for.
			name: "running with a container not ready",
			pod: corev1.Pod{
				Spec:   corev1.PodSpec{Containers: containers(2)},
				Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{running(true, 0), running(false, 3)}},
			},
			wantStatus: "Running", wantReady: "1/2", wantRestarts: 3,
		},
		{
			name: "stuck pulling an image",
			pod: corev1.Pod{
				Spec:   corev1.PodSpec{Containers: containers(1)},
				Status: corev1.PodStatus{Phase: corev1.PodPending, ContainerStatuses: []corev1.ContainerStatus{waiting("ImagePullBackOff")}},
			},
			wantStatus: "ImagePullBackOff", wantReady: "0/1",
		},
		{
			name: "crash looping",
			pod: corev1.Pod{
				Spec:   corev1.PodSpec{Containers: containers(1)},
				Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{waiting("CrashLoopBackOff")}},
			},
			wantStatus: "CrashLoopBackOff", wantReady: "0/1",
		},
		{
			// An unfinished init container blocks everything after it, and is
			// prefixed so it is not mistaken for the main container's state.
			name: "waiting on an init container",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase:                 corev1.PodPending,
					InitContainerStatuses: []corev1.ContainerStatus{waiting("CrashLoopBackOff")},
					ContainerStatuses:     []corev1.ContainerStatus{waiting("PodInitializing")},
				},
			},
			wantStatus: "Init:CrashLoopBackOff", wantReady: "0/1",
		},
		{
			// A deleted pod keeps its phase until it is gone, so the phase would
			// report Running for something on its way out.
			name: "terminating",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &deleted},
				Spec:       corev1.PodSpec{Containers: containers(1)},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{running(true, 0)}},
			},
			wantStatus: "Terminating", wantReady: "1/1",
		},
		{
			// A finished job's container is Terminated with reason Completed,
			// which is not a problem and must not be reported as the status.
			name: "a completed pod",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
					ContainerStatuses: []corev1.ContainerStatus{{
						State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed"}},
					}},
				},
			},
			wantStatus: "Succeeded", wantReady: "0/1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := summarizePod(&test.pod)
			if got.Status != test.wantStatus {
				t.Errorf("Status = %q, want %q", got.Status, test.wantStatus)
			}
			if got.Ready != test.wantReady {
				t.Errorf("Ready = %q, want %q", got.Ready, test.wantReady)
			}
			if got.Restarts != test.wantRestarts {
				t.Errorf("Restarts = %d, want %d", got.Restarts, test.wantRestarts)
			}
		})
	}
}

// A crash-looping init container is exactly the pod whose restart count an
// operator is reading, and its restarts belong in the total. Readiness does not
// include it: a finished init container is not serving anything.
func TestSummarizePodCountsInitContainerRestarts(t *testing.T) {
	pod := corev1.Pod{
		Spec: corev1.PodSpec{Containers: containers(1)},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
			InitContainerStatuses: []corev1.ContainerStatus{
				{RestartCount: 5, State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
			},
			ContainerStatuses: []corev1.ContainerStatus{waiting("PodInitializing")},
		},
	}

	got := summarizePod(&pod)
	if got.Restarts != 5 {
		t.Errorf("Restarts = %d, want 5 from the init container", got.Restarts)
	}
	if got.Ready != "0/1" {
		t.Errorf("Ready = %q, want 0/1 — an init container is not a serving container", got.Ready)
	}
	if got.Status != "Init:CrashLoopBackOff" {
		t.Errorf("Status = %q, want the init container's reason", got.Status)
	}
}

func terminated(reason string, exitCode int32) corev1.ContainerStatus {
	return corev1.ContainerStatus{State: corev1.ContainerState{
		Terminated: &corev1.ContainerStateTerminated{Reason: reason, ExitCode: exitCode}}}
}

// The Reason on a terminated container is optional. Reading only that field
// reports a pod whose container was killed as Running, which is the opposite of
// what happened.
func TestSummarizePodReportsTerminationsWithNoReason(t *testing.T) {
	tests := []struct {
		name   string
		status corev1.ContainerStatus
		want   string
	}{
		{
			name:   "a reason is used when there is one",
			status: terminated("OOMKilled", 137),
			want:   "OOMKilled",
		},
		{
			// The case that was invisible: something killed the container and set
			// no reason, so only the exit code says anything happened.
			name:   "no reason falls back to the exit code",
			status: terminated("", 137),
			want:   "ExitCode:137",
		},
		{
			// A container that finished its work is not blocking anything, whether
			// or not it said so.
			name:   "a clean exit with no reason is not a failure",
			status: terminated("", 0),
			want:   "Running",
		},
		{
			name:   "Completed is not a failure either",
			status: terminated("Completed", 0),
			want:   "Running",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pod := corev1.Pod{
				Spec: corev1.PodSpec{Containers: containers(1)},
				Status: corev1.PodStatus{
					Phase:             corev1.PodRunning,
					ContainerStatuses: []corev1.ContainerStatus{test.status},
				},
			}
			if got := summarizePod(&pod).Status; got != test.want {
				t.Errorf("status = %q, want %q", got, test.want)
			}
		})
	}
}

// An init container that failed is reported with its prefix, so the exit-code
// fallback has to carry it too.
func TestSummarizePodPrefixesInitTerminationExitCodes(t *testing.T) {
	pod := corev1.Pod{
		Spec: corev1.PodSpec{Containers: containers(1), InitContainers: containers(1)},
		Status: corev1.PodStatus{
			Phase:                 corev1.PodPending,
			InitContainerStatuses: []corev1.ContainerStatus{terminated("", 2)},
			ContainerStatuses:     []corev1.ContainerStatus{waiting("PodInitializing")},
		},
	}
	if got := summarizePod(&pod).Status; got != "Init:ExitCode:2" {
		t.Errorf("status = %q, want %q", got, "Init:ExitCode:2")
	}
}

// The images a workload runs include its init containers'. Several things in this
// cluster do their migration or config rendering in one, so leaving them out
// makes "which version is deployed" unanswerable for exactly those.
func TestWorkloadIncludesInitContainerImages(t *testing.T) {
	spec := corev1.PodSpec{
		InitContainers: []corev1.Container{{Name: "migrate", Image: "registry.invalid/migrate:2"}},
		Containers: []corev1.Container{
			{Name: "app", Image: "registry.invalid/app:1"},
			{Name: "sidecar", Image: "registry.invalid/sidecar:3"},
		},
	}

	got := workload("Deployment", metav1.ObjectMeta{Name: "api"}, 1, 1, spec).Images
	want := []string{
		"registry.invalid/app:1",
		"registry.invalid/sidecar:3",
		// Appended after the main ones: the question is usually about those.
		"registry.invalid/migrate:2",
	}

	if len(got) != len(want) {
		t.Fatalf("images = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("images[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
