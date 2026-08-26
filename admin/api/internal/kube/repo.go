package kube

import (
	"context"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// eventLimit bounds an event listing. A namespace under repeated failure produces
// thousands, and the useful ones are the most recent — but the API returns them in
// arbitrary order, so they are sorted after fetching rather than truncated before.
const eventLimit = 100

// Repository reads the cluster through client-go.
type Repository struct {
	client kubernetes.Interface
}

// NewRepository wraps a clientset.
func NewRepository(client kubernetes.Interface) *Repository {
	return &Repository{client: client}
}

// ListNamespaces returns every namespace.
func (r *Repository) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	list, err := r.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translate(err, "list namespaces")
	}

	namespaces := make([]Namespace, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		namespaces = append(namespaces, Namespace{
			Name:   item.Name,
			Status: string(item.Status.Phase),
			Age:    item.CreationTimestamp.Time,
		})
	}
	sort.Slice(namespaces, func(a, b int) bool { return namespaces[a].Name < namespaces[b].Name })
	return namespaces, nil
}

// ListWorkloads returns the Deployments, StatefulSets, and DaemonSets in one
// namespace as a single list, because the question is what is running rather than
// which kind of controller is running it.
func (r *Repository) ListWorkloads(ctx context.Context, namespace string) ([]Workload, error) {
	workloads := []Workload{}

	deployments, err := r.client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translate(err, "list deployments")
	}
	for i := range deployments.Items {
		item := &deployments.Items[i]
		desired := int32(1)
		if item.Spec.Replicas != nil {
			desired = *item.Spec.Replicas
		}
		workloads = append(workloads, workload("Deployment", item.ObjectMeta,
			desired, item.Status.ReadyReplicas, item.Spec.Template.Spec))
	}

	statefulSets, err := r.client.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translate(err, "list statefulsets")
	}
	for i := range statefulSets.Items {
		item := &statefulSets.Items[i]
		desired := int32(1)
		if item.Spec.Replicas != nil {
			desired = *item.Spec.Replicas
		}
		workloads = append(workloads, workload("StatefulSet", item.ObjectMeta,
			desired, item.Status.ReadyReplicas, item.Spec.Template.Spec))
	}

	daemonSets, err := r.client.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translate(err, "list daemonsets")
	}
	for i := range daemonSets.Items {
		item := &daemonSets.Items[i]
		// A DaemonSet's desired count comes from how many nodes it matches rather
		// than from a configured replica count.
		workloads = append(workloads, workload("DaemonSet", item.ObjectMeta,
			item.Status.DesiredNumberScheduled, item.Status.NumberReady, item.Spec.Template.Spec))
	}

	sort.Slice(workloads, func(a, b int) bool {
		if workloads[a].Kind != workloads[b].Kind {
			return workloads[a].Kind < workloads[b].Kind
		}
		return workloads[a].Name < workloads[b].Name
	})
	return workloads, nil
}

// workload builds the common shape from one controller's metadata and template.
func workload(kind string, meta metav1.ObjectMeta, desired, ready int32, spec corev1.PodSpec) Workload {
	images := make([]string, 0, len(spec.Containers))
	for i := range spec.Containers {
		images = append(images, spec.Containers[i].Image)
	}
	return Workload{
		Kind:      kind,
		Name:      meta.Name,
		Namespace: meta.Namespace,
		Desired:   desired,
		Ready:     ready,
		Images:    images,
		CreatedAt: meta.CreationTimestamp.Time,
	}
}

// ListPods returns the pods in one namespace.
func (r *Repository) ListPods(ctx context.Context, namespace string) ([]Pod, error) {
	list, err := r.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translate(err, "list pods")
	}

	pods := make([]Pod, 0, len(list.Items))
	for i := range list.Items {
		pods = append(pods, summarizePod(&list.Items[i]))
	}
	sort.Slice(pods, func(a, b int) bool { return pods[a].Name < pods[b].Name })
	return pods, nil
}

// ListEvents returns the recent events in one namespace, most recent first.
func (r *Repository) ListEvents(ctx context.Context, namespace string) ([]Event, error) {
	list, err := r.client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translate(err, "list events")
	}

	events := make([]Event, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		events = append(events, Event{
			Type:      item.Type,
			Reason:    item.Reason,
			Message:   item.Message,
			Object:    fmt.Sprintf("%s/%s", item.InvolvedObject.Kind, item.InvolvedObject.Name),
			Count:     item.Count,
			LastSeen:  lastSeen(item),
			Namespace: item.Namespace,
		})
	}

	sort.Slice(events, func(a, b int) bool { return events[a].LastSeen.After(events[b].LastSeen) })
	if len(events) > eventLimit {
		events = events[:eventLimit]
	}
	return events, nil
}

// lastSeen prefers the repeat timestamp and falls back to the first. A one-off
// event has no LastTimestamp, and reporting a zero time would sort it to the end.
func lastSeen(event *corev1.Event) time.Time {
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	return event.FirstTimestamp.Time
}

// translate turns an API error into one of this slice's, so the handler can map
// it without importing Kubernetes' error package.
func translate(err error, what string) error {
	switch {
	case apierrors.IsNotFound(err):
		return fmt.Errorf("%w: %s", ErrNotFound, what)
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		// The panel's ServiceAccount is bound to a read-only role. A forbidden
		// reply usually means that binding is missing rather than that the caller
		// did anything wrong, so it is worth distinguishing from a 500.
		return fmt.Errorf("%w: %s", ErrForbidden, what)
	default:
		return fmt.Errorf("%s: %w", what, err)
	}
}
