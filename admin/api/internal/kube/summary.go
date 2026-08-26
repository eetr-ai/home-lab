package kube

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReadSummary returns the dashboard's rollup in one round trip.
//
// The arithmetic is in rollUp, which takes what was read rather than reading it,
// so the counting has tests without a cluster.
func (r *Repository) ReadSummary(ctx context.Context) (Summary, error) {
	pods, err := r.allPods(ctx)
	if err != nil {
		return Summary{}, err
	}

	nodes, measured, err := r.nodesWithLoad(ctx, pods)
	if err != nil {
		return Summary{}, err
	}

	workloads, err := r.ListWorkloads(ctx, metav1.NamespaceAll)
	if err != nil {
		return Summary{}, err
	}

	claims, err := r.listVolumeClaims(ctx)
	if err != nil {
		return Summary{}, err
	}

	namespaces, err := r.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return Summary{}, translate(err, "list namespaces")
	}

	summarized := make([]Pod, 0, len(pods))
	for i := range pods {
		summarized = append(summarized, summarizePod(&pods[i]))
	}

	return rollUp(nodes, summarized, workloads, claims, len(namespaces.Items), measured), nil
}

// rollUp counts what the dashboard reports.
func rollUp(
	nodes []Node, pods []Pod, workloads []Workload, claims []VolumeClaim,
	namespaces int, measured bool,
) Summary {
	return Summary{
		Nodes:            rollUpNodes(nodes),
		Pods:             rollUpPods(pods),
		Workloads:        rollUpWorkloads(workloads),
		Storage:          rollUpStorage(claims),
		Namespaces:       namespaces,
		MetricsAvailable: measured,
	}
}

// rollUpNodes sums the cluster's capacity and what is claimed against it.
//
// Allocatable rather than capacity, because capacity includes what the kubelet
// and the OS have reserved and nothing can ever schedule into it.
func rollUpNodes(nodes []Node) NodeSummary {
	summary := NodeSummary{Total: len(nodes)}
	for i := range nodes {
		node := &nodes[i]
		if node.Ready {
			summary.Ready++
		}
		if len(node.Pressure) > 0 {
			summary.Pressure++
		}
		summary.Allocatable = summary.Allocatable.add(node.Allocatable)
		summary.Requested = summary.Requested.add(node.Requested)
		if node.Usage != nil {
			summary.Usage = summary.Usage.add(*node.Usage)
		}
	}
	return summary
}

// rollUpPods counts pods by the states worth acting on.
func rollUpPods(pods []Pod) PodSummary {
	summary := PodSummary{Total: len(pods)}
	for i := range pods {
		pod := &pods[i]
		summary.Restarts += int(pod.Restarts)
		switch pod.Phase {
		case string(corev1.PodRunning):
			summary.Running++
		case string(corev1.PodPending):
			summary.Pending++
		case string(corev1.PodFailed):
			summary.Failed++
		}
	}
	return summary
}

// rollUpWorkloads counts controllers and how many are short of their replicas.
//
// A workload scaled deliberately to zero is not degraded: it wants none and has
// none, which is the state it was put in rather than a fault.
func rollUpWorkloads(workloads []Workload) WorkloadSummary {
	summary := WorkloadSummary{Total: len(workloads)}
	for i := range workloads {
		if workloads[i].Ready < workloads[i].Desired {
			summary.Degraded++
		}
	}
	return summary
}

// rollUpStorage sums the claims, counting capacity only where it exists.
//
// Anything not Bound counts as unbound rather than as pending: a Lost claim —
// one whose volume went away — is neither bound nor on its way to being, and
// filing it under "pending" would suggest it is still coming.
func rollUpStorage(claims []VolumeClaim) StorageSummary {
	summary := StorageSummary{Claims: len(claims)}
	for i := range claims {
		claim := &claims[i]
		if claim.Status != string(corev1.ClaimBound) {
			summary.Unbound++
			continue
		}
		summary.CapacityBytes += claim.CapacityBytes
	}
	return summary
}
