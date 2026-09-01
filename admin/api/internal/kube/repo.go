package kube

import (
	"context"
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	metricsclient "k8s.io/metrics/pkg/client/clientset/versioned"
)

// eventLimit bounds an event listing. A namespace under repeated failure produces
// thousands, and the useful ones are the most recent — but the API returns them in
// arbitrary order, so they are sorted after fetching rather than truncated before.
const eventLimit = 100

// Repository reads the cluster through client-go.
type Repository struct {
	client kubernetes.Interface
	// streamClient is the same cluster with no request deadline, used only for
	// log streaming. See NewStreamClientset for why it cannot be the one above.
	streamClient kubernetes.Interface
	// metrics is the optional metrics.k8s.io client. Nil when metrics-server is
	// not expected, and every read through it degrades to "no reading" rather
	// than to an error — see nodeUsage.
	metrics metricsclient.Interface
	// nodeStats switches on reading node disk usage from the kubelet, which needs
	// a grant the panel does not hold by default. See nodeFilesystem.
	nodeStats bool
}

// NewRepository wraps the clients this reads the cluster with.
func NewRepository(
	client, streamClient kubernetes.Interface, metrics metricsclient.Interface, nodeStats bool,
) *Repository {
	return &Repository{
		client:       client,
		streamClient: streamClient,
		metrics:      metrics,
		nodeStats:    nodeStats,
	}
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
		namespaces = append(namespaces, namespaceFrom(item))
	}
	sort.Slice(namespaces, func(a, b int) bool { return namespaces[a].Name < namespaces[b].Name })
	return namespaces, nil
}

// ReadNamespace returns one namespace.
func (r *Repository) ReadNamespace(ctx context.Context, name string) (Namespace, error) {
	item, err := r.client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return Namespace{}, translate(err, "read namespace "+name)
	}
	return namespaceFrom(item), nil
}

// CreateNamespace creates a namespace with the labels it was given.
//
// The labels arrive already decided: the service applied its own over the
// caller's, which is where that rule belongs. This puts the object on the cluster
// and nothing else.
func (r *Repository) CreateNamespace(ctx context.Context, spec NamespaceSpec) (Namespace, error) {
	item, err := r.client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: spec.Name, Labels: spec.Labels},
	}, metav1.CreateOptions{})
	if err != nil {
		return Namespace{}, translate(err, "create namespace "+spec.Name)
	}
	return namespaceFrom(item), nil
}

// DeleteNamespace deletes a namespace and everything in it.
//
// Deletion is asynchronous: the API server marks the namespace Terminating and
// its own controller removes the contents, which can take a while and can stall
// on a finalizer. So a successful return means the deletion was accepted, not
// that it finished — the namespace stays in the listing, in Terminating, until it
// does.
// DeleteNamespace deletes the namespace, but only if it is still the object the
// caller checked.
//
// The UID goes in as a precondition rather than being trusted from the name
// alone: between reading a namespace and deleting it, that name can be deleted
// and recreated by something else, and the second object may not be one this
// panel was allowed to touch. With the precondition the API server refuses the
// delete instead, which is the right answer to "the thing I checked is gone".
//
// An empty UID means the caller had none to offer, and the delete proceeds
// unconditioned rather than failing.
func (r *Repository) DeleteNamespace(ctx context.Context, name, uid string) error {
	options := metav1.DeleteOptions{}
	if uid != "" {
		objectUID := types.UID(uid)
		options.Preconditions = &metav1.Preconditions{UID: &objectUID}
	}

	return translate(
		r.client.CoreV1().Namespaces().Delete(ctx, name, options),
		"delete namespace "+name)
}

// namespaceFrom translates the cluster's namespace into this slice's.
//
// Whether it is protected is deliberately not filled in here: that is a rule, and
// rules live in the service.
func namespaceFrom(item *corev1.Namespace) Namespace {
	return Namespace{
		Name:   item.Name,
		UID:    string(item.UID),
		Status: string(item.Status.Phase),
		Age:    item.CreationTimestamp.Time,
		Labels: item.Labels,
	}
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
//
// Init-container images are included, after the main ones. They are part of what
// the workload runs, and leaving them out makes "which version is deployed"
// unanswerable for anything that does its migration or its config rendering in an
// init container — which several things here do.
func workload(kind string, meta metav1.ObjectMeta, desired, ready int32, spec corev1.PodSpec) Workload {
	images := make([]string, 0, len(spec.Containers)+len(spec.InitContainers))
	for i := range spec.Containers {
		images = append(images, spec.Containers[i].Image)
	}
	for i := range spec.InitContainers {
		images = append(images, spec.InitContainers[i].Image)
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
	case err == nil:
		// Load-bearing: callers pass a write's error straight through, and the
		// default branch below would turn a nil into a non-nil error wrapping
		// nothing — a successful restart reported as a failure.
		return nil
	case apierrors.IsNotFound(err):
		return fmt.Errorf("%w: %s", ErrNotFound, what)
	case apierrors.IsConflict(err):
		return fmt.Errorf("%w: %s", ErrConflict, what)
	case apierrors.IsAlreadyExists(err):
		return fmt.Errorf("%w: %s", ErrAlreadyExists, what)
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		// The panel's ServiceAccount is bound to a read-only role. A forbidden
		// reply usually means that binding is missing rather than that the caller
		// did anything wrong, so it is worth distinguishing from a 500.
		return fmt.Errorf("%w: %s", ErrForbidden, what)
	default:
		return fmt.Errorf("%s: %w", what, err)
	}
}

// CreateSecret writes a new Opaque Secret, and fails if one of that name is
// already there.
//
// StringData rather than Data: client-go base64-encodes it on the way out, so
// nothing here has to, and a value cannot be stored double-encoded by mistake.
func (r *Repository) CreateSecret(ctx context.Context, namespace string, spec SecretSpec) error {
	_, err := r.client.CoreV1().Secrets(namespace).Create(ctx, secretObject(namespace, spec),
		metav1.CreateOptions{})
	return translate(err, "create secret "+namespace+"/"+spec.Name)
}

// UpdateSecret writes the Secret whether or not it is there.
//
// An apply-style update rather than a read-modify-write: the caller asked for
// this exact content, and merging it with whatever is already in the object would
// leave keys from a previous credential beside the new one — which is a Secret
// holding two passwords and no way to tell which is live.
func (r *Repository) UpdateSecret(ctx context.Context, namespace string, spec SecretSpec) error {
	object := secretObject(namespace, spec)
	_, err := r.client.CoreV1().Secrets(namespace).Update(ctx, object, metav1.UpdateOptions{})
	if apierrors.IsNotFound(err) {
		// Overwrite means "make it say this", and there being nothing to replace
		// is not a failure to report to somebody who asked for that.
		_, err = r.client.CoreV1().Secrets(namespace).Create(ctx, object, metav1.CreateOptions{})
	}
	return translate(err, "write secret "+namespace+"/"+spec.Name)
}

// secretObject builds the object both writes send.
func secretObject(namespace string, spec SecretSpec) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.Name,
			Namespace: namespace,
			Labels:    spec.Labels,
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: spec.Data,
	}
}

// ListSecrets reports every Secret in a namespace, carrying none of their values.
//
// The projection happens here rather than in the service, and that placement is
// the guarantee: the live objects with their data never leave this function, so
// no later layer can serialise one by adding a field. What comes out cannot hold
// a value because SecretSummary has nowhere to put one.
func (r *Repository) ListSecrets(ctx context.Context, namespace string) ([]SecretSummary, error) {
	list, err := r.client.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, translate(err, "list secrets in "+namespace)
	}

	secrets := make([]SecretSummary, 0, len(list.Items))
	for i := range list.Items {
		secrets = append(secrets, summarizeSecret(&list.Items[i]))
	}
	// By name, because the API server's order is by nothing an operator can see
	// and a list that reshuffles between reloads is a list you cannot scan.
	sort.Slice(secrets, func(i, j int) bool { return secrets[i].Name < secrets[j].Name })
	return secrets, nil
}

// ReadSecret reports one Secret, again without its values. It exists for the
// write paths, which have to know a Secret's type and keys before they can decide
// whether they may touch it.
func (r *Repository) ReadSecret(ctx context.Context, namespace, name string) (SecretSummary, error) {
	secret, err := r.client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return SecretSummary{}, translate(err, "read secret "+namespace+"/"+name)
	}
	return summarizeSecret(secret), nil
}

// DeleteSecret removes the Secret that was read and found deletable, and not
// merely one of that name.
//
// The preconditions are what make that sentence true. The service reads a Secret,
// checks its type against the deny-list, and then asks for it to be removed; in
// between, that name could have come to mean a different object — a Helm release
// Secret restored from a backup, say. Naming the UID and the resource version
// turns that race into a 409 the operator sees instead of a deletion nobody
// asked for. The window is narrow and the cost of closing it is two fields.
func (r *Repository) DeleteSecret(ctx context.Context, namespace string, target SecretSummary) error {
	uid := types.UID(target.uid)
	err := r.client.CoreV1().Secrets(namespace).Delete(ctx, target.Name, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{
			UID:             &uid,
			ResourceVersion: &target.resourceVersion,
		},
	})
	return translate(err, "delete secret "+namespace+"/"+target.Name)
}

// RotateSecretKeys replaces the values of the named keys and leaves the rest.
//
// A read-modify-write, which is the one place in this file that is deliberately
// not the apply-style update UpdateSecret uses. The difference is what the caller
// knows: an overwrite is somebody saying "make it say exactly this", while a
// rotation is somebody who cannot see the other keys and must not disturb them.
//
// Under RetryOnConflict because the window between the read and the write is real
// and the loss would be silent — the second writer's rotation would answer 409
// and the operator would reasonably read that as "nothing happened", which is
// true but leaves them holding a password that is not installed anywhere.
//
// Data rather than StringData, because the object read back has its values in
// Data and setting both leaves StringData winning for the keys it names and Data
// for the rest — which works, and is a rule nobody should have to know.
func (r *Repository) RotateSecretKeys(
	ctx context.Context, namespace, name string, values map[string]string,
) error {
	secrets := r.client.CoreV1().Secrets(namespace)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		live, err := secrets.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("read for rotation: %w", err)
		}
		// Re-checked against this read, not against the one the service made.
		// Every trip round this loop is a fresh object, and a retry provoked by a
		// conflict is precisely the case where the object changed under us — so
		// the guards that decided this Secret was rotatable have to decide it
		// again about the thing actually being written.
		if err = rotatable(live, values); err != nil {
			return err
		}
		if live.Data == nil {
			live.Data = map[string][]byte{}
		}
		for key, value := range values {
			live.Data[key] = []byte(value)
		}
		// StringData is cleared rather than trusted to be empty: a Secret written
		// by an apply carries none, but this is the object the API server will
		// take literally and leaving a field unexamined here is how a value from
		// somewhere else wins.
		live.StringData = nil
		// Wrapped with %w rather than returned bare: RetryOnConflict decides
		// whether to go round again with apierrors.IsConflict, which unwraps, so
		// the retry still sees a conflict for what it is.
		if _, err = secrets.Update(ctx, live, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("write for rotation: %w", err)
		}
		return nil
	})
	return translate(err, "rotate secret "+namespace+"/"+name)
}

// rotatable repeats the service's verdict about the object that is about to be
// written, and returns an error RetryOnConflict will not retry.
//
// It duplicates three checks the service already made, and that duplication is
// the point: the service made them about an object it read a moment ago, and the
// only object whose type, mutability and keys matter is the one this Update is
// going to replace.
func rotatable(live *corev1.Secret, values map[string]string) error {
	if reserved, reason := reservedSecret(string(live.Type)); reserved {
		return fmt.Errorf("%w: %s is %s", ErrReserved, live.Name, reason)
	}
	if live.Immutable != nil && *live.Immutable {
		return fmt.Errorf("%w: %s is immutable", ErrReserved, live.Name)
	}
	for key := range values {
		if _, has := live.Data[key]; !has {
			return fmt.Errorf("%w: %s has no key %q", ErrInvalidName, live.Name, key)
		}
	}
	return nil
}

// summarizeSecret is the projection, and the only place a live Secret is turned
// into something this API will send.
func summarizeSecret(secret *corev1.Secret) SecretSummary {
	keys := make([]string, 0, len(secret.Data)+len(secret.StringData))
	for key := range secret.Data {
		keys = append(keys, key)
	}
	// StringData is normally empty on a Secret read back — the API server folds it
	// into Data — but it is checked so a key cannot go unreported on the one that
	// has not been through a round trip yet.
	for key := range secret.StringData {
		if _, already := secret.Data[key]; !already {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	return SecretSummary{
		Name:         secret.Name,
		Type:         string(secret.Type),
		Keys:         keys,
		Immutable:    secret.Immutable != nil && *secret.Immutable,
		PanelManaged: secret.Labels[labelManagedBy] == managedByValue,
		CreatedAt:    secret.CreationTimestamp.Time,

		uid:             string(secret.UID),
		resourceVersion: secret.ResourceVersion,
	}
}
