package kube

import (
	"context"
	"fmt"
	"time"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// restartAnnotation is where a rollout restart is recorded.
//
// kubectl's own key, deliberately: a restart the panel triggers should be
// indistinguishable from one an operator triggered at a terminal, and both should
// show up the same way in `kubectl describe`.
const restartAnnotation = "kubectl.kubernetes.io/restartedAt"

// RestartWorkload rolls a workload's pods.
//
// A restart is a patch of the pod template's annotations: changing the template
// makes the controller consider its pods out of date and replace them under its
// own rollout strategy. Nothing is deleted here — the workload's maxUnavailable
// still governs how many pods go at once.
//
// The timestamp comes from the caller rather than from time.Now() here, so the
// value written is testable.
func (r *Repository) RestartWorkload(
	ctx context.Context, kind, namespace, name string, at time.Time,
) error {
	patch := restartPatch(at)

	switch kind {
	case KindDeployment:
		_, err := r.client.AppsV1().Deployments(namespace).
			Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
		return translate(err, "restart the deployment")
	case KindStatefulSet:
		_, err := r.client.AppsV1().StatefulSets(namespace).
			Patch(ctx, name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
		return translate(err, "restart the statefulset")
	default:
		return fmt.Errorf("%w: %s cannot be restarted", ErrUnsupportedKind, kind)
	}
}

// restartPatch is the only patch this API ever sends to a workload.
//
// Worth stating plainly, because RBAC cannot: `patch` on a Deployment is patch on
// any field of it, so what actually confines this to a restart is this function.
// See the note in charts/admin/templates/api/rbac.yaml.
func restartPatch(at time.Time) []byte {
	return fmt.Appendf(nil,
		`{"spec":{"template":{"metadata":{"annotations":{%q:%q}}}}}`,
		restartAnnotation, at.UTC().Format(time.RFC3339))
}

// ReadScale returns a workload's current and desired replica counts.
func (r *Repository) ReadScale(ctx context.Context, kind, namespace, name string) (Scale, error) {
	scale, err := r.getScale(ctx, kind, namespace, name)
	if err != nil {
		return Scale{}, err
	}
	return Scale{Replicas: scale.Spec.Replicas, Current: scale.Status.Replicas}, nil
}

// ScaleWorkload sets a workload's replica count.
//
// Read then write rather than a blind update: the scale subresource carries a
// resourceVersion, and sending one that is stale is how two operators scaling at
// once end up with the second silently overwriting the first. The API server
// refuses the conflicting write instead.
func (r *Repository) ScaleWorkload(
	ctx context.Context, kind, namespace, name string, replicas int32,
) error {
	scale, err := r.getScale(ctx, kind, namespace, name)
	if err != nil {
		return err
	}
	scale.Spec.Replicas = replicas

	switch kind {
	case KindDeployment:
		_, err = r.client.AppsV1().Deployments(namespace).
			UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	case KindStatefulSet:
		_, err = r.client.AppsV1().StatefulSets(namespace).
			UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	default:
		return fmt.Errorf("%w: %s cannot be scaled", ErrUnsupportedKind, kind)
	}
	return translate(err, "scale the workload")
}

// getScale reads the scale subresource for whichever kind this is.
//
// A DaemonSet has none: its replica count comes from how many nodes it matches,
// so there is nothing to set.
func (r *Repository) getScale(
	ctx context.Context, kind, namespace, name string,
) (*autoscalingv1.Scale, error) {
	apps := r.client.AppsV1()

	var (
		scale *autoscalingv1.Scale
		err   error
	)
	switch kind {
	case KindDeployment:
		scale, err = apps.Deployments(namespace).GetScale(ctx, name, metav1.GetOptions{})
	case KindStatefulSet:
		scale, err = apps.StatefulSets(namespace).GetScale(ctx, name, metav1.GetOptions{})
	default:
		return nil, fmt.Errorf("%w: %s has no replica count to read", ErrUnsupportedKind, kind)
	}
	if err != nil {
		return nil, translate(err, "read the workload's scale")
	}
	return scale, nil
}
