package kube

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ReadWorkload returns one workload and everything around it.
//
// The pods, services, and claims are found by following the workload's own
// selector rather than by name, because that is the only relationship Kubernetes
// actually models: a Service reaches a workload's pods when its selector matches
// their labels, and nothing records that it was meant to.
func (r *Repository) ReadWorkload(
	ctx context.Context, kind, namespace, name string,
) (WorkloadDetail, error) {
	detail, selector, err := r.readController(ctx, kind, namespace, name)
	if err != nil {
		return WorkloadDetail{}, err
	}

	found, err := r.podsMatching(ctx, namespace, selector)
	if err != nil {
		return WorkloadDetail{}, err
	}
	detail.Pods = found.pods

	services, err := r.servicesFor(ctx, namespace, found.labels)
	if err != nil {
		return WorkloadDetail{}, err
	}
	detail.Services = services

	claims, err := r.claimsFor(ctx, namespace, found.claims)
	if err != nil {
		return WorkloadDetail{}, err
	}
	detail.Claims = claims

	events, err := r.eventsFor(ctx, namespace, name, found.pods)
	if err != nil {
		return WorkloadDetail{}, err
	}
	detail.Events = events

	return detail, nil
}

// readController fetches the workload itself and returns its pod selector.
func (r *Repository) readController(
	ctx context.Context, kind, namespace, name string,
) (WorkloadDetail, labels.Selector, error) {
	apps := r.client.AppsV1()

	switch kind {
	case KindDeployment:
		found, err := apps.Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return WorkloadDetail{}, nil, translate(err, "read the deployment")
		}
		return deploymentDetail(found), selectorOf(found.Spec.Selector), nil
	case KindStatefulSet:
		found, err := apps.StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return WorkloadDetail{}, nil, translate(err, "read the statefulset")
		}
		return statefulSetDetail(found), selectorOf(found.Spec.Selector), nil
	case KindDaemonSet:
		found, err := apps.DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return WorkloadDetail{}, nil, translate(err, "read the daemonset")
		}
		return daemonSetDetail(found), selectorOf(found.Spec.Selector), nil
	default:
		return WorkloadDetail{}, nil, fmt.Errorf("%w: %s", ErrUnsupportedKind, kind)
	}
}

func deploymentDetail(found *appsv1.Deployment) WorkloadDetail {
	desired := int32(1)
	if found.Spec.Replicas != nil {
		desired = *found.Spec.Replicas
	}
	return WorkloadDetail{
		Workload: workload(KindDeployment, found.ObjectMeta,
			desired, found.Status.ReadyReplicas, found.Spec.Template.Spec),
		Scale:      &Scale{Replicas: desired, Current: found.Status.Replicas},
		Updated:    found.Status.UpdatedReplicas,
		Available:  found.Status.AvailableReplicas,
		Conditions: deploymentConditions(found.Status.Conditions),
	}
}

func statefulSetDetail(found *appsv1.StatefulSet) WorkloadDetail {
	desired := int32(1)
	if found.Spec.Replicas != nil {
		desired = *found.Spec.Replicas
	}
	return WorkloadDetail{
		Workload: workload(KindStatefulSet, found.ObjectMeta,
			desired, found.Status.ReadyReplicas, found.Spec.Template.Spec),
		Scale:      &Scale{Replicas: desired, Current: found.Status.Replicas},
		Updated:    found.Status.UpdatedReplicas,
		Available:  found.Status.AvailableReplicas,
		Conditions: statefulSetConditions(found.Status.Conditions),
	}
}

// daemonSetDetail reports no Scale: a DaemonSet's count is however many nodes it
// matches, so there is nothing a caller could set.
func daemonSetDetail(found *appsv1.DaemonSet) WorkloadDetail {
	return WorkloadDetail{
		Workload: workload(KindDaemonSet, found.ObjectMeta,
			found.Status.DesiredNumberScheduled, found.Status.NumberReady, found.Spec.Template.Spec),
		Updated:    found.Status.UpdatedNumberScheduled,
		Available:  found.Status.NumberAvailable,
		Conditions: daemonSetConditions(found.Status.Conditions),
	}
}

// selectorOf turns a label selector into one that can match, treating a missing
// or malformed selector as matching nothing rather than everything.
//
// The difference matters: treating it as Everything would report every pod in the
// namespace as belonging to this workload. Note that the callers cannot detect
// this by asking the result — labels.Nothing() and labels.Everything() both
// render as "" — so they test the rendered string instead.
func selectorOf(selector *metav1.LabelSelector) labels.Selector {
	if selector == nil {
		return labels.Nothing()
	}
	converted, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return labels.Nothing()
	}
	// A selector with no requirements converts to Everything, which would claim
	// every pod in the namespace. Normalise it here so the sentinel is honest
	// rather than relying on every caller to notice.
	if converted.Empty() {
		return labels.Nothing()
	}
	return converted
}

// matched is what one listing of a workload's pods yields.
//
// Three things from a single API call, because all three come from the same
// objects. Fetching them separately meant either a second listing or a Get per
// pod.
type matched struct {
	pods []Pod
	// labels is each pod's own label set. Kept because a Service is matched by
	// evaluating its selector against real pod labels — see servicesFor.
	labels []labels.Set
	// claims names the volume claims these pods mount.
	claims map[string]struct{}
}

// podsMatching returns the pods a selector covers, with their labels and claims.
//
// The guard tests the rendered selector, not Empty(). Both sentinels render as
// the empty string — and labels.Nothing() reports Empty() as *false*, so an
// Empty() check passes it straight through to List, which reads an empty
// LabelSelector as "everything". A workload with no selector would then claim
// every pod in the namespace as its own.
//
// The labels and claims come from the same listing because summarizePod discards
// both, and recovering them any other way meant a Get per pod on every detail
// page load — plus a "the pod vanished between the list and the read" case that
// now cannot arise.
func (r *Repository) podsMatching(
	ctx context.Context, namespace string, selector labels.Selector,
) (matched, error) {
	found := matched{pods: []Pod{}, labels: []labels.Set{}, claims: map[string]struct{}{}}
	if selector.String() == "" {
		return found, nil
	}

	list, err := r.client.CoreV1().Pods(namespace).
		List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return matched{}, translate(err, "list the workload's pods")
	}

	for i := range list.Items {
		item := &list.Items[i]
		found.pods = append(found.pods, summarizePod(item))
		found.labels = append(found.labels, labels.Set(item.Labels))
		for j := range item.Spec.Volumes {
			if claim := item.Spec.Volumes[j].PersistentVolumeClaim; claim != nil {
				found.claims[claim.ClaimName] = struct{}{}
			}
		}
	}
	sort.Slice(found.pods, func(a, b int) bool { return found.pods[a].Name < found.pods[b].Name })
	return found, nil
}

// servicesFor returns the services that reach this workload's pods.
//
// Each Service's selector is evaluated against the pods' actual labels. The
// earlier version rebuilt an equality label set out of the *workload's* selector
// and matched against that, which got two cases quietly wrong: a multi-value In
// requirement was dropped, and a one-value NotIn became an equality label —
// asserting the opposite of what it required. Testing against the labels the
// pods really carry has neither problem, and is what Kubernetes itself does.
func (r *Repository) servicesFor(
	ctx context.Context, namespace string, podLabels []labels.Set,
) ([]ClusterService, error) {
	if len(podLabels) == 0 {
		return []ClusterService{}, nil
	}

	list, err := r.client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translate(err, "list the workload's services")
	}

	services := []ClusterService{}
	for i := range list.Items {
		item := &list.Items[i]
		// A Service with no selector routes to manually managed endpoints and
		// belongs to no workload here.
		if len(item.Spec.Selector) == 0 {
			continue
		}
		if reaches(labels.SelectorFromSet(item.Spec.Selector), podLabels) {
			services = append(services, summarizeService(item))
		}
	}
	sort.Slice(services, func(a, b int) bool { return services[a].Name < services[b].Name })
	return services, nil
}

// reaches reports whether a Service's selector matches any of these pods.
//
// Any rather than all: a Service reaching some of a workload's pods still reaches
// the workload, and mid-rollout the old and new pods differ by at least the
// pod-template-hash label.
func reaches(selector labels.Selector, podLabels []labels.Set) bool {
	for _, set := range podLabels {
		if selector.Matches(set) {
			return true
		}
	}
	return false
}

func summarizeService(service *corev1.Service) ClusterService {
	ports := make([]string, 0, len(service.Spec.Ports))
	for i := range service.Spec.Ports {
		port := &service.Spec.Ports[i]
		ports = append(ports, fmt.Sprintf("%d→%s/%s",
			port.Port, port.TargetPort.String(), port.Protocol))
	}

	selector := make([]string, 0, len(service.Spec.Selector))
	for key, value := range service.Spec.Selector {
		selector = append(selector, key+"="+value)
	}
	sort.Strings(selector)

	return ClusterService{
		Name:      service.Name,
		Namespace: service.Namespace,
		Type:      string(service.Spec.Type),
		ClusterIP: service.Spec.ClusterIP,
		Ports:     ports,
		Selector:  selector,
	}
}

// claimsFor returns the volume claims named by a workload's pods.
//
// From the pods rather than from a StatefulSet's volumeClaimTemplates, because
// the pods are where both cases meet: a template produces one claim per pod with
// a generated name, and a Deployment mounts a claim somebody made by hand.
func (r *Repository) claimsFor(
	ctx context.Context, namespace string, mounted map[string]struct{},
) ([]VolumeClaim, error) {
	if len(mounted) == 0 {
		return []VolumeClaim{}, nil
	}

	list, err := r.client.CoreV1().PersistentVolumeClaims(namespace).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translate(err, "list the workload's volume claims")
	}

	claims := []VolumeClaim{}
	for i := range list.Items {
		if _, ok := mounted[list.Items[i].Name]; ok {
			claims = append(claims, summarizeClaim(&list.Items[i]))
		}
	}
	sort.Slice(claims, func(a, b int) bool { return claims[a].Name < claims[b].Name })
	return claims, nil
}

// eventsFor returns the events about this workload and its pods.
//
// Both, because they say different things: the controller's events explain why a
// rollout has not started, and the pods' explain why it has not finished.
func (r *Repository) eventsFor(
	ctx context.Context, namespace, name string, pods []Pod,
) ([]Event, error) {
	all, err := r.ListEvents(ctx, namespace)
	if err != nil {
		return nil, err
	}

	interesting := map[string]struct{}{name: {}}
	for i := range pods {
		interesting[pods[i].Name] = struct{}{}
	}

	events := []Event{}
	for i := range all {
		// Object is "Kind/name"; only the name identifies which thing it is about.
		_, objectName, found := strings.Cut(all[i].Object, "/")
		if !found {
			continue
		}
		if _, ok := interesting[objectName]; ok {
			events = append(events, all[i])
		}
	}
	return events, nil
}

// The three controller kinds report their conditions as three different types
// that carry the same five fields, so each gets a two-line conversion rather than
// the whole slice growing a generic one.

func deploymentConditions(conditions []appsv1.DeploymentCondition) []Condition {
	out := make([]Condition, 0, len(conditions))
	for i := range conditions {
		condition := &conditions[i]
		out = append(out, Condition{
			Type:           string(condition.Type),
			Status:         string(condition.Status),
			Reason:         condition.Reason,
			Message:        condition.Message,
			LastTransition: condition.LastTransitionTime.Time,
		})
	}
	return out
}

func statefulSetConditions(conditions []appsv1.StatefulSetCondition) []Condition {
	out := make([]Condition, 0, len(conditions))
	for i := range conditions {
		condition := &conditions[i]
		out = append(out, Condition{
			Type:           string(condition.Type),
			Status:         string(condition.Status),
			Reason:         condition.Reason,
			Message:        condition.Message,
			LastTransition: condition.LastTransitionTime.Time,
		})
	}
	return out
}

func daemonSetConditions(conditions []appsv1.DaemonSetCondition) []Condition {
	out := make([]Condition, 0, len(conditions))
	for i := range conditions {
		condition := &conditions[i]
		out = append(out, Condition{
			Type:           string(condition.Type),
			Status:         string(condition.Status),
			Reason:         condition.Reason,
			Message:        condition.Message,
			LastTransition: condition.LastTransitionTime.Time,
		})
	}
	return out
}
