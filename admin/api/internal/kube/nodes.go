package kube

import (
	"context"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// rolePrefix is where Kubernetes records a node's role. There is no field for it;
// the labels are the only source, and kubectl reads them the same way.
const rolePrefix = "node-role.kubernetes.io/"

// unschedulable is appended to a cordoned node's status, because a cordoned node
// is Ready and still cannot take work — which the conditions alone do not say.
const unschedulable = "Ready,SchedulingDisabled"

// ListNodes returns every node, with what is scheduled against it and, when
// metrics-server answers, what is actually being used.
func (r *Repository) ListNodes(ctx context.Context) ([]Node, error) {
	pods, err := r.allPods(ctx)
	if err != nil {
		return nil, err
	}
	nodes, _, err := r.nodesWithLoad(ctx, pods)
	return nodes, err
}

// allPods lists every pod in the cluster, which is what both the node report and
// the dashboard summary are computed from.
func (r *Repository) allPods(ctx context.Context) ([]corev1.Pod, error) {
	list, err := r.client.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translate(err, "list pods")
	}
	return list.Items, nil
}

// nodesWithLoad builds the node report from an already-fetched pod list, and
// reports whether the metrics API answered.
//
// Usage is best effort by design. metrics-server is an optional component that
// may be absent, restarting, or not yet have collected its first sample, and a
// dashboard that fails entirely because live CPU is briefly unavailable is worse
// than one that says so for that one figure. The bool is how the caller tells
// "nothing is running" from "nothing measured it".
func (r *Repository) nodesWithLoad(ctx context.Context, pods []corev1.Pod) ([]Node, bool, error) {
	list, err := r.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, false, translate(err, "list nodes")
	}

	requested := requestsByNode(pods)
	usage, measured := r.nodeUsage(ctx)

	nodes := make([]Node, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		node := summarizeNode(item, requested[item.Name])
		if reading, ok := usage[item.Name]; ok {
			node.Usage = &reading
		}
		if stats, ok := r.nodeFilesystem(ctx, item.Name); ok {
			node.Filesystem = &stats
		}
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(a, b int) bool { return nodes[a].Name < nodes[b].Name })
	return nodes, measured, nil
}

// nodeUsage reads live consumption, reporting nothing rather than failing.
//
// Every error is swallowed on purpose — the metrics API being absent is the
// expected state on a cluster without metrics-server, and it arrives as the same
// 404 as a missing node.
func (r *Repository) nodeUsage(ctx context.Context) (map[string]Resources, bool) {
	if r.metrics == nil {
		return nil, false
	}

	list, err := r.metrics.MetricsV1beta1().NodeMetricses().List(ctx, metav1.ListOptions{})
	if err != nil {
		r.logMetricsUnavailable(err, "node metrics")
		return nil, false
	}

	usage := make(map[string]Resources, len(list.Items))
	for i := range list.Items {
		usage[list.Items[i].Name] = resourcesFrom(list.Items[i].Usage)
	}
	return usage, true
}

// summarizeNode builds the reported shape from one node and its scheduled load.
func summarizeNode(node *corev1.Node, requested Resources) Node {
	status, ready := nodeStatus(node)
	return Node{
		Name:        node.Name,
		Status:      status,
		Ready:       ready,
		Roles:       nodeRoles(node.Labels),
		Version:     node.Status.NodeInfo.KubeletVersion,
		OS:          nodeOS(node),
		Pressure:    nodePressure(node),
		Capacity:    resourcesFrom(node.Status.Capacity),
		Allocatable: resourcesFrom(node.Status.Allocatable),
		Requested:   requested,
		CreatedAt:   node.CreationTimestamp.Time,
	}
}

// nodeStatus reports the node's readiness the way kubectl does.
func nodeStatus(node *corev1.Node) (string, bool) {
	ready := false
	status := "Unknown"
	for i := range node.Status.Conditions {
		condition := &node.Status.Conditions[i]
		if condition.Type != corev1.NodeReady {
			continue
		}
		ready = condition.Status == corev1.ConditionTrue
		status = "NotReady"
		if ready {
			status = "Ready"
		}
	}
	// A cordoned node is Ready and takes no new work. Reporting only "Ready" would
	// leave an operator wondering why nothing schedules onto it.
	if node.Spec.Unschedulable && ready {
		return unschedulable, ready
	}
	return status, ready
}

// nodeRoles reads the roles out of the node-role.kubernetes.io/* labels.
func nodeRoles(labels map[string]string) []string {
	roles := []string{}
	for key := range labels {
		if name, found := strings.CutPrefix(key, rolePrefix); found && name != "" {
			roles = append(roles, name)
		}
	}
	sort.Strings(roles)
	return roles
}

// nodeOS describes the machine, for telling one node from another at a glance.
func nodeOS(node *corev1.Node) string {
	info := node.Status.NodeInfo
	parts := []string{}
	for _, part := range []string{info.OSImage, info.Architecture} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " / ")
}

// nodePressure lists the conditions that are true and should not be.
//
// Only the pressure conditions, and only when true: Ready is reported separately,
// and a list of every condition a healthy node holds says nothing.
func nodePressure(node *corev1.Node) []string {
	pressure := []string{}
	for i := range node.Status.Conditions {
		condition := &node.Status.Conditions[i]
		if condition.Type == corev1.NodeReady || condition.Status != corev1.ConditionTrue {
			continue
		}
		pressure = append(pressure, string(condition.Type))
	}
	sort.Strings(pressure)
	return pressure
}

// resourcesFrom converts a Kubernetes resource list into the reported units.
func resourcesFrom(list corev1.ResourceList) Resources {
	resources := Resources{}
	if cpu, ok := list[corev1.ResourceCPU]; ok {
		resources.CPUMillis = cpu.MilliValue()
	}
	if memory, ok := list[corev1.ResourceMemory]; ok {
		resources.MemoryBytes = memory.Value()
	}
	if pods, ok := list[corev1.ResourcePods]; ok {
		resources.Pods = pods.Value()
	}
	if ephemeral, ok := list[corev1.ResourceEphemeralStorage]; ok {
		resources.EphemeralBytes = ephemeral.Value()
	}
	return resources
}

// add sums two quantities, so a rollup does not repeat the field list.
func (r Resources) add(other Resources) Resources {
	return Resources{
		CPUMillis:      r.CPUMillis + other.CPUMillis,
		MemoryBytes:    r.MemoryBytes + other.MemoryBytes,
		Pods:           r.Pods + other.Pods,
		EphemeralBytes: r.EphemeralBytes + other.EphemeralBytes,
	}
}

// max keeps the larger of each field, which is how init containers combine.
func (r Resources) max(other Resources) Resources {
	return Resources{
		CPUMillis:      maxInt64(r.CPUMillis, other.CPUMillis),
		MemoryBytes:    maxInt64(r.MemoryBytes, other.MemoryBytes),
		Pods:           maxInt64(r.Pods, other.Pods),
		EphemeralBytes: maxInt64(r.EphemeralBytes, other.EphemeralBytes),
	}
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// requestsByNode sums what the pods on each node have reserved.
//
// Terminated pods are skipped: a Succeeded or Failed pod still has a nodeName and
// still appears in a list, and counting its requests would report a node as full
// of work that finished.
func requestsByNode(pods []corev1.Pod) map[string]Resources {
	requested := make(map[string]Resources)
	for i := range pods {
		pod := &pods[i]
		if pod.Spec.NodeName == "" || isTerminated(pod) {
			continue
		}
		requested[pod.Spec.NodeName] = requested[pod.Spec.NodeName].add(podRequests(pod))
	}
	return requested
}

// isTerminated reports a pod that has finished and holds nothing.
func isTerminated(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed
}

// podRequests is a pod's effective request, the same arithmetic the scheduler does.
//
// Three rules, and each one exists because a simpler sum gets a real pod wrong:
//
//   - Pod-level resources, when set, are the answer. They are a ceiling the pod
//     is admitted against, and the containers' own requests do not add to them.
//   - A plain init container runs to completion before the others start, so the
//     pod needs whichever is larger: the main containers, or that init container.
//     Summing all of them overstates every pod that migrates in an init container.
//   - A restartable init container — a sidecar — runs for the pod's whole life, so
//     it does add. It is also held by every later init container, which is why it
//     accumulates rather than being compared.
//
// This mirrors k8s.io/component-helpers' PodRequests. It is reimplemented rather
// than imported because that helper takes options this does not need and would
// pull a second Kubernetes module in for one function.
func podRequests(pod *corev1.Pod) Resources {
	if pod.Spec.Resources != nil {
		if podLevel := resourcesFrom(pod.Spec.Resources.Requests); podLevel != (Resources{}) {
			return withOverhead(podLevel, pod)
		}
	}

	total := Resources{}
	for i := range pod.Spec.Containers {
		total = total.add(resourcesFrom(pod.Spec.Containers[i].Resources.Requests))
	}

	// sidecars is what the restartable init containers hold once they are up, and
	// initPeak the most any single init container needs while it runs — which
	// includes every sidecar started before it.
	sidecars := Resources{}
	initPeak := Resources{}
	for i := range pod.Spec.InitContainers {
		container := &pod.Spec.InitContainers[i]
		request := resourcesFrom(container.Resources.Requests)
		if isSidecar(container) {
			sidecars = sidecars.add(request)
			total = total.add(request)
			initPeak = initPeak.max(sidecars)
			continue
		}
		initPeak = initPeak.max(request.add(sidecars))
	}

	return withOverhead(total.max(initPeak), pod)
}

// isSidecar reports an init container that keeps running alongside the others.
func isSidecar(container *corev1.Container) bool {
	return container.RestartPolicy != nil &&
		*container.RestartPolicy == corev1.ContainerRestartPolicyAlways
}

// withOverhead adds what the runtime itself costs, which the pod also reserves.
func withOverhead(requests Resources, pod *corev1.Pod) Resources {
	if pod.Spec.Overhead == nil {
		return requests
	}
	return requests.add(resourcesFrom(pod.Spec.Overhead))
}
