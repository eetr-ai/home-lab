package kube

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func quantities(cpu, memory string) corev1.ResourceList {
	list := corev1.ResourceList{}
	if cpu != "" {
		list[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if memory != "" {
		list[corev1.ResourceMemory] = resource.MustParse(memory)
	}
	return list
}

func requesting(cpu, memory string) corev1.Container {
	return corev1.Container{Resources: corev1.ResourceRequirements{Requests: quantities(cpu, memory)}}
}

func sidecar(cpu, memory string) corev1.Container {
	always := corev1.ContainerRestartPolicyAlways
	container := requesting(cpu, memory)
	container.RestartPolicy = &always
	return container
}

func condition(kind corev1.NodeConditionType, status corev1.ConditionStatus) corev1.NodeCondition {
	return corev1.NodeCondition{Type: kind, Status: status}
}

// A cordoned node is Ready and takes no work, which is the case the status
// summary exists to distinguish.
func TestNodeStatus(t *testing.T) {
	tests := []struct {
		name      string
		node      corev1.Node
		want      string
		wantReady bool
	}{
		{
			name: "a healthy node",
			node: corev1.Node{Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{condition(corev1.NodeReady, corev1.ConditionTrue)}}},
			want: "Ready", wantReady: true,
		},
		{
			name: "a node that is not ready",
			node: corev1.Node{Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{condition(corev1.NodeReady, corev1.ConditionFalse)}}},
			want: "NotReady",
		},
		{
			name: "a cordoned node is ready and unschedulable",
			node: corev1.Node{
				Spec: corev1.NodeSpec{Unschedulable: true},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{condition(corev1.NodeReady, corev1.ConditionTrue)}},
			},
			want: unschedulable, wantReady: true,
		},
		{
			// A cordoned node that is also down is down. Reporting it as
			// SchedulingDisabled would understate the problem.
			name: "a cordoned node that is not ready",
			node: corev1.Node{
				Spec: corev1.NodeSpec{Unschedulable: true},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{condition(corev1.NodeReady, corev1.ConditionFalse)}},
			},
			want: "NotReady",
		},
		{
			name: "a node reporting no readiness at all",
			node: corev1.Node{Status: corev1.NodeStatus{}},
			want: "Unknown",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ready := nodeStatus(&test.node)
			if got != test.want || ready != test.wantReady {
				t.Errorf("nodeStatus() = %q, %t; want %q, %t", got, ready, test.want, test.wantReady)
			}
		})
	}
}

// Only the pressure conditions, and only the true ones: a list of every condition
// a healthy node holds says nothing.
func TestNodePressure(t *testing.T) {
	node := corev1.Node{Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
		condition(corev1.NodeReady, corev1.ConditionTrue),
		condition(corev1.NodeMemoryPressure, corev1.ConditionFalse),
		condition(corev1.NodeDiskPressure, corev1.ConditionTrue),
		condition(corev1.NodePIDPressure, corev1.ConditionTrue),
	}}}

	got := nodePressure(&node)
	want := []string{"DiskPressure", "PIDPressure"}
	if len(got) != len(want) {
		t.Fatalf("nodePressure() = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("nodePressure()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

func TestNodeRoles(t *testing.T) {
	got := nodeRoles(map[string]string{
		rolePrefix + "worker":        "",
		rolePrefix + "control-plane": "",
		"kubernetes.io/hostname":     "node-1",
		rolePrefix:                   "", // the bare prefix names no role
	})

	want := []string{"control-plane", "worker"}
	if len(got) != len(want) {
		t.Fatalf("nodeRoles() = %v; want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("nodeRoles()[%d] = %q; want %q", i, got[i], want[i])
		}
	}
}

// Init containers run to completion before the others start, so a pod needs
// whichever is larger rather than the sum. Summing would overstate every pod that
// migrates in an init container.
func TestPodRequests(t *testing.T) {
	tests := []struct {
		name       string
		pod        corev1.Pod
		wantCPU    int64
		wantMemory int64
	}{
		{
			name: "containers are summed",
			pod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{requesting("100m", "128Mi"), requesting("250m", "256Mi")}}},
			wantCPU: 350, wantMemory: 128*1024*1024 + 256*1024*1024,
		},
		{
			name: "a larger init container wins over the sum of the others",
			pod: corev1.Pod{Spec: corev1.PodSpec{
				Containers:     []corev1.Container{requesting("100m", "128Mi")},
				InitContainers: []corev1.Container{requesting("500m", "64Mi")}}},
			wantCPU: 500, wantMemory: 128 * 1024 * 1024,
		},
		{
			name: "only the largest init container counts, not their sum",
			pod: corev1.Pod{Spec: corev1.PodSpec{
				InitContainers: []corev1.Container{requesting("100m", ""), requesting("300m", "")}}},
			wantCPU: 300,
		},
		{
			name: "overhead is added on top",
			pod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{requesting("100m", "")},
				Overhead:   quantities("50m", "")}},
			wantCPU: 150,
		},
		{
			// A sidecar runs for the pod's whole life rather than finishing first,
			// so it adds instead of competing. Treating it like a plain init
			// container would understate every pod with a mesh proxy or a log
			// shipper — which is the common case for one.
			name: "a restartable init container adds to the total",
			pod: corev1.Pod{Spec: corev1.PodSpec{
				Containers:     []corev1.Container{requesting("100m", "128Mi")},
				InitContainers: []corev1.Container{sidecar("200m", "64Mi")}}},
			wantCPU: 300, wantMemory: 128*1024*1024 + 64*1024*1024,
		},
		{
			// The plain init container runs after the sidecar is up, so it holds
			// both at once — and that peak is what has to fit.
			name: "a plain init container is held alongside the sidecars before it",
			pod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{requesting("50m", "")},
				InitContainers: []corev1.Container{
					sidecar("200m", ""),
					requesting("400m", ""),
				}}},
			// Peak is 400m + 200m = 600m while migrating; steady state is only
			// 50m + 200m. The larger one is what the node must have.
			wantCPU: 600,
		},
		{
			// Pod-level resources are a ceiling the pod is admitted against. The
			// containers' own requests do not add to them.
			name: "pod-level resources win over the containers",
			pod: corev1.Pod{Spec: corev1.PodSpec{
				Containers: []corev1.Container{requesting("100m", "128Mi"), requesting("100m", "128Mi")},
				Resources:  &corev1.ResourceRequirements{Requests: quantities("1", "1Gi")}}},
			wantCPU: 1000, wantMemory: 1024 * 1024 * 1024,
		},
		{
			name: "a pod that requests nothing reserves nothing",
			pod:  corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{}}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := podRequests(&test.pod)
			if got.CPUMillis != test.wantCPU {
				t.Errorf("CPUMillis = %d; want %d", got.CPUMillis, test.wantCPU)
			}
			if got.MemoryBytes != test.wantMemory {
				t.Errorf("MemoryBytes = %d; want %d", got.MemoryBytes, test.wantMemory)
			}
		})
	}
}

// A finished pod still has a nodeName and still appears in a list. Counting its
// requests would report a node as full of work that is over.
func TestRequestsByNodeSkipsTerminatedPods(t *testing.T) {
	pods := []corev1.Pod{
		{
			Spec:   corev1.PodSpec{NodeName: "node-1", Containers: []corev1.Container{requesting("100m", "")}},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			Spec:   corev1.PodSpec{NodeName: "node-1", Containers: []corev1.Container{requesting("400m", "")}},
			Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
		},
		{
			Spec:   corev1.PodSpec{NodeName: "node-1", Containers: []corev1.Container{requesting("800m", "")}},
			Status: corev1.PodStatus{Phase: corev1.PodFailed},
		},
		{
			// Unscheduled: it reserves nothing anywhere yet.
			Spec:   corev1.PodSpec{Containers: []corev1.Container{requesting("999m", "")}},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
	}

	got := requestsByNode(pods)
	if got["node-1"].CPUMillis != 100 {
		t.Errorf("node-1 CPUMillis = %d; want 100", got["node-1"].CPUMillis)
	}
	if len(got) != 1 {
		t.Errorf("requestsByNode() covered %d nodes; want 1", len(got))
	}
}
