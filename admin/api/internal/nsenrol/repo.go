package nsenrol

import (
	"context"
	"fmt"
	"slices"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/eetr-ai/home-lab/admin/api/internal/nspolicy"
)

// Repository is the cluster access enrolment needs: RoleBindings, and the
// namespaces carrying the Helm label.
//
// It reads namespaces through a label selector rather than listing them all and
// filtering here. One request, and the API server does the matching — which also
// means this holds no opinion about namespaces that are not candidates.
type Repository struct {
	client kubernetes.Interface
}

// NewRepository wraps the cluster client.
func NewRepository(client kubernetes.Interface) *Repository {
	return &Repository{client: client}
}

// ListBindings returns the panel's own RoleBindings in a namespace.
//
// Only the panel's: the names are known, so anything else in the namespace is
// none of this code's business and is not read.
func (r *Repository) ListBindings(ctx context.Context, namespace string, names []string) ([]Binding, error) {
	list, err := r.client.RbacV1().RoleBindings(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list role bindings in %s: %w", namespace, err)
	}

	bindings := make([]Binding, 0, len(names))
	for i := range list.Items {
		item := &list.Items[i]
		if !slices.Contains(names, item.Name) {
			continue
		}
		bindings = append(bindings, bindingFrom(item))
	}
	return bindings, nil
}

// ListAllBindings returns the panel's own RoleBindings across every namespace,
// grouped by namespace.
//
// One request rather than one per namespace. A namespace listing shows enrolment
// beside every row, and asking the API server per row would turn a page into a
// dozen round trips for an answer that arrives in one.
func (r *Repository) ListAllBindings(ctx context.Context, names []string) (map[string][]Binding, error) {
	list, err := r.client.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list role bindings: %w", err)
	}

	grouped := map[string][]Binding{}
	for i := range list.Items {
		item := &list.Items[i]
		if !slices.Contains(names, item.Name) {
			continue
		}
		grouped[item.Namespace] = append(grouped[item.Namespace], bindingFrom(item))
	}
	return grouped, nil
}

// CreateBinding creates one RoleBinding, replacing nothing.
func (r *Repository) CreateBinding(ctx context.Context, namespace string, binding Binding) error {
	object := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      binding.Name,
			Namespace: namespace,
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "home-lab-admin"},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     binding.RoleRefKind,
			Name:     binding.RoleRef,
		},
	}
	for _, subject := range binding.Subjects {
		account, namespaceOf, ok := splitSubject(subject)
		if !ok {
			return fmt.Errorf("malformed subject %q", subject)
		}
		object.Subjects = append(object.Subjects, rbacv1.Subject{
			Kind:      rbacv1.ServiceAccountKind,
			Name:      account,
			Namespace: namespaceOf,
		})
	}

	_, err := r.client.RbacV1().RoleBindings(namespace).Create(ctx, object, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create role binding %s/%s: %w", namespace, binding.Name, err)
	}
	return nil
}

// DeleteBinding removes one, so a wrong roleRef can be replaced. There is no
// editing one: roleRef is immutable.
func (r *Repository) DeleteBinding(ctx context.Context, namespace, name string) error {
	err := r.client.RbacV1().RoleBindings(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		// Somebody else got there first, which is the state this wanted.
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete role binding %s/%s: %w", namespace, name, err)
	}
	return nil
}

// ListCandidates returns the namespaces carrying the Helm label, with their
// labels, so the caller can apply policy to them.
func (r *Repository) ListCandidates(ctx context.Context) ([]Candidate, error) {
	list, err := r.client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: nspolicy.LabelManaged + "=true",
	})
	if err != nil {
		return nil, fmt.Errorf("list managed namespaces: %w", err)
	}

	candidates := make([]Candidate, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		candidates = append(candidates, Candidate{Name: item.Name, Labels: item.Labels})
	}
	return candidates, nil
}

// Candidate is a namespace that says it is a Helm target. Whether it really is
// one is still the policy's answer, not the label's.
type Candidate struct {
	Name   string
	Labels map[string]string
}

// bindingFrom reduces a live RoleBinding to what correctness is decided from.
func bindingFrom(item *rbacv1.RoleBinding) Binding {
	binding := Binding{
		Name:        item.Name,
		RoleRefKind: item.RoleRef.Kind,
		RoleRef:     item.RoleRef.Name,
	}
	for _, subject := range item.Subjects {
		if subject.Kind != rbacv1.ServiceAccountKind {
			continue
		}
		binding.Subjects = append(binding.Subjects, subject.Namespace+"/"+subject.Name)
	}
	return binding
}

// splitSubject reads a "namespace/name" ServiceAccount reference.
func splitSubject(subject string) (name, namespace string, ok bool) {
	namespace, name, found := strings.Cut(subject, "/")
	if !found || namespace == "" || name == "" {
		return "", "", false
	}
	return name, namespace, true
}
