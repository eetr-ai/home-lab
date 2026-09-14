package kube

import (
	"fmt"
	"slices"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// A cluster-wide listing has to group by namespace, and the only thing making it
// do so is the sort.
//
// Worth asserting because the failure is a quiet one: the API server returns
// items in an order nobody promised, so a listing sorted by name alone reads as
// "sorted" until two namespaces hold names that interleave — which, for pods
// named after the charts that made them, they routinely do. The rows would then
// be a cluster-wide list whose namespace column jumps about.
func TestPodsListClusterWideGroupedByNamespace(t *testing.T) {
	client := fake.NewClientset(
		pod("apps", "zookeeper-0"),
		pod("admin", "admin-web-1"),
		pod("apps", "aardvark-0"),
		pod("admin", "admin-api-0"),
	)
	repo := NewRepository(client, client, nil, false)

	pods, err := repo.ListPods(t.Context(), AllNamespaces)
	if err != nil {
		t.Fatalf("ListPods error = %v", err)
	}

	got := make([]string, 0, len(pods))
	for _, one := range pods {
		got = append(got, fmt.Sprintf("%s/%s", one.Namespace, one.Name))
	}
	want := []string{
		"admin/admin-api-0",
		"admin/admin-web-1",
		"apps/aardvark-0",
		"apps/zookeeper-0",
	}
	if !slices.Equal(got, want) {
		t.Errorf("pods = %v, want %v", got, want)
	}
}

// Workloads sort by namespace, then kind, then name — the three kinds are one
// list, so without the kind in the middle a namespace's Deployments and its
// StatefulSets interleave under a column that says which is which.
func TestWorkloadsListClusterWideGroupedByNamespaceThenKind(t *testing.T) {
	client := fake.NewClientset(
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "mongo"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "podinfo"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: "admin", Name: "admin-api"}},
	)
	repo := NewRepository(client, client, nil, false)

	workloads, err := repo.ListWorkloads(t.Context(), AllNamespaces)
	if err != nil {
		t.Fatalf("ListWorkloads error = %v", err)
	}

	got := make([]string, 0, len(workloads))
	for _, one := range workloads {
		got = append(got, fmt.Sprintf("%s/%s/%s", one.Namespace, one.Kind, one.Name))
	}
	want := []string{
		"admin/Deployment/admin-api",
		"apps/Deployment/podinfo",
		"apps/StatefulSet/mongo",
	}
	if !slices.Equal(got, want) {
		t.Errorf("workloads = %v, want %v", got, want)
	}
}

func pod(namespace, name string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}}
}
