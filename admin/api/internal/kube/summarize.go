package kube

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
)

// summarizePod turns a pod into the row an operator reads.
//
// The phase alone is not enough, and that is the whole reason this exists.
// A pod stuck pulling an image is phase Pending with a container waiting on
// ImagePullBackOff; a pod being deleted stays phase Running until it is gone; and
// a pod whose containers have crashed reports Running while serving nothing. The
// phase is reported too, but the summary is what answers the question.
func summarizePod(pod *corev1.Pod) Pod {
	ready := int32(0)
	restarts := int32(0)
	for i := range pod.Status.ContainerStatuses {
		status := &pod.Status.ContainerStatuses[i]
		if status.Ready {
			ready++
		}
		restarts += status.RestartCount
	}
	// Init container restarts count too. A pod whose init container is crash
	// looping is exactly the one whose restart count an operator is looking at,
	// and leaving them out reported it as zero. Readiness stays limited to the
	// regular containers, because a finished init container is not serving.
	for i := range pod.Status.InitContainerStatuses {
		restarts += pod.Status.InitContainerStatuses[i].RestartCount
	}

	return Pod{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Phase:     string(pod.Status.Phase),
		Status:    podStatus(pod),
		Ready:     fmt.Sprintf("%d/%d", ready, len(pod.Spec.Containers)),
		Restarts:  restarts,
		Node:      pod.Spec.NodeName,
		CreatedAt: pod.CreationTimestamp.Time,
	}
}

// podStatus is the most specific thing that can be said about a pod.
func podStatus(pod *corev1.Pod) string {
	// A deleted pod keeps its phase until it goes, so the phase would say Running
	// for something on its way out.
	if pod.DeletionTimestamp != nil {
		return "Terminating"
	}

	// A waiting or crashed container is the reason the pod is not working, and it
	// is never the phase. Init containers first: while one is unfinished, nothing
	// in the pod proper has started.
	if reason := blockedContainerReason(pod.Status.InitContainerStatuses, "Init:"); reason != "" {
		return reason
	}
	if reason := blockedContainerReason(pod.Status.ContainerStatuses, ""); reason != "" {
		return reason
	}
	return string(pod.Status.Phase)
}

// blockedContainerReason returns the first reason a container is not running,
// prefixed for init containers so Init:CrashLoopBackOff is distinguishable.
func blockedContainerReason(statuses []corev1.ContainerStatus, prefix string) string {
	for i := range statuses {
		state := statuses[i].State
		switch {
		case state.Waiting != nil && state.Waiting.Reason != "":
			return prefix + state.Waiting.Reason
		case state.Terminated != nil:
			if reason := terminationReason(state.Terminated); reason != "" {
				return prefix + reason
			}
		}
	}
	return ""
}

// terminationReason names a termination worth reporting, or "" for one that is
// not — a container that finished its work.
//
// The Reason field is optional in the API, so a container killed by something
// that set no reason arrives with an empty string and a non-zero exit code. Read
// only the reason and that pod reports itself as Running, which is the opposite of
// what happened. kubectl falls back to the exit code for exactly this case, and so
// does this.
func terminationReason(state *corev1.ContainerStateTerminated) string {
	if state.Reason != "" {
		if state.Reason == "Completed" {
			return ""
		}
		return state.Reason
	}
	if state.ExitCode != 0 {
		return "ExitCode:" + strconv.Itoa(int(state.ExitCode))
	}
	return ""
}
