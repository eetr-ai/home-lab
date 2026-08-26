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

	pods, mounted, err := r.podsMatching(ctx, namespace, selector)
	if err != nil {
		return WorkloadDetail{}, err
	}
	detail.Pods = pods

	services, err := r.servicesFor(ctx, namespace, selector)
	if err != nil {
		return WorkloadDetail{}, err
	}
	detail.Services = services

	claims, err := r.claimsFor(ctx, namespace, mounted)
	if err != nil {
		return WorkloadDetail{}, err
	}
	detail.Claims = claims

	events, err := r.eventsFor(ctx, namespace, name, pods)
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

// podsMatching returns the pods a selector covers.
//
// The guard tests the rendered selector, not Empty(). Both sentinels render as
// the empty string — and labels.Nothing() reports Empty() as *false*, so an
// Empty() check passes it straight through to List, which reads an empty
// LabelSelector as "everything". A workload with no selector would then claim
// every pod in the namespace as its own.
// It also returns the volume claims those pods mount, gathered from the same
// listing. The pods carry their own Spec.Volumes and summarizePod discards them,
// so recovering the claim names any other way meant one Get per pod on every
// detail page load — and a "the pod vanished between the list and the read" case
// that now cannot arise.
func (r *Repository) podsMatching(
	ctx context.Context, namespace string, selector labels.Selector,
) ([]Pod, map[string]struct{}, error) {
	mounted := map[string]struct{}{}
	if selector.String() == "" {
		return []Pod{}, mounted, nil
	}

	list, err := r.client.CoreV1().Pods(namespace).
		List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return nil, nil, translate(err, "list the workload's pods")
	}

	pods := make([]Pod, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		pods = append(pods, summarizePod(item))
		for j := range item.Spec.Volumes {
			if claim := item.Spec.Volumes[j].PersistentVolumeClaim; claim != nil {
				mounted[claim.ClaimName] = struct{}{}
			}
		}
	}
	sort.Slice(pods, func(a, b int) bool { return pods[a].Name < pods[b].Name })
	return pods, mounted, nil
}

// servicesFor returns the services whose selector reaches this workload's pods.
//
// The test runs the other way round from podsMatching: a Service selects pods, so
// what matters is whether the Service's selector is satisfied by the workload's
// labels — not whether the two selectors are equal, which they often are not.
func (r *Repository) servicesFor(
	ctx context.Context, namespace string, selector labels.Selector,
) ([]ClusterService, error) {
	// Same sentinel trap as podsMatching, checked before asking the cluster.
	if selector.String() == "" {
		return []ClusterService{}, nil
	}

	list, err := r.client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translate(err, "list the workload's services")
	}

	workloadLabels := labelsFromSelector(selector)
	services := []ClusterService{}
	for i := range list.Items {
		item := &list.Items[i]
		// A Service with no selector routes to manually managed endpoints and
		// belongs to no workload here.
		if len(item.Spec.Selector) == 0 {
			continue
		}
		if labels.SelectorFromSet(item.Spec.Selector).Matches(workloadLabels) {
			services = append(services, summarizeService(item))
		}
	}
	sort.Slice(services, func(a, b int) bool { return services[a].Name < services[b].Name })
	return services, nil
}

// labelsFromSelector recovers the equality-based labels a selector requires, so
// they can be tested against another selector.
func labelsFromSelector(selector labels.Selector) labels.Set {
	set := labels.Set{}
	requirements, _ := selector.Requirements()
	for _, requirement := range requirements {
		values := requirement.Values().List()
		if len(values) == 1 {
			set[requirement.Key()] = values[0]
		}
	}
	return set
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
