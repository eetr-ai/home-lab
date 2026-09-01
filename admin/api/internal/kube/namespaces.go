package kube

import (
	"context"
	"fmt"
	"strings"

	"github.com/eetr-ai/home-lab/admin/api/internal/nspolicy"
)

// The labels the panel puts on a namespace it creates.
const (
	// labelManagedBy records who made it, so a namespace created here is
	// distinguishable from one created by hand or by another tool.
	labelManagedBy = "app.kubernetes.io/managed-by"
	// managedByValue is this panel.
	managedByValue = "home-lab-admin"
	// labelPodSecurity is the enforcing half of Pod Security admission.
	//
	// This is the only mechanism in the whole design that constrains what a
	// workload in the namespace may *do*, as opposed to what manifests may be
	// created. RBAC cannot express "create a Deployment, but not a privileged
	// one"; admission can, and it is applied at creation because a namespace that
	// starts without it is one nobody remembers to fix.
	labelPodSecurity = "pod-security.kubernetes.io/enforce"
	// defaultPodSecurity is the level applied when none is configured. Baseline
	// rather than restricted: restricted refuses containers that do not declare a
	// non-root user, which is most upstream charts, and a level that has to be
	// turned off to install anything is a level that gets turned off.
	defaultPodSecurity = "baseline"
)

// podSecurityLevels are the only values Kubernetes accepts on the enforce label.
//
// Anything else is refused by the API server, so a level that is not one of these
// does not degrade to something weaker — it makes every namespace creation fail.
// NewService checks against this at startup so that failure arrives once, at a
// moment somebody is watching, rather than on each attempt.
var podSecurityLevels = []string{"privileged", "baseline", "restricted"}

// reservedLabelSuffixes are the label-key domains a caller may not write into.
//
// Kubernetes reserves these for itself and for admission controllers, and one of
// them decides whether a pod may run privileged. Letting a caller set an
// arbitrary label at creation would make "create a namespace" mean "grant myself
// pod-security.kubernetes.io/enforce: privileged", which is the opposite of what
// creating one through this panel is for.
var reservedLabelSuffixes = []string{"kubernetes.io", "k8s.io"}

// ReadNamespace returns one namespace and whether the panel may delete it.
func (s *Service) ReadNamespace(ctx context.Context, name string) (Namespace, error) {
	if err := validateNamespace(name); err != nil {
		return Namespace{}, err
	}

	namespace, err := s.repo.ReadNamespace(ctx, name)
	if err != nil {
		return Namespace{}, err
	}
	s.applyPolicy(&namespace)
	return namespace, nil
}

// CreateNamespace brings a namespace into existence with the labels this lab
// requires on one.
//
// The caller supplies a name and, optionally, labels of its own; the ones below
// are applied over them, so a request cannot opt out of Pod Security admission by
// naming the same key. A caller may set home-lab.example/gateway-access and
// home-lab.example/redis-access, which are the two the install scripts apply by
// hand today and which a namespace serving anything will need.
//
// The name is checked before the labels, and both before anything reaches the
// cluster: a refused request must not have created a namespace and then failed.
func (s *Service) CreateNamespace(ctx context.Context, spec NamespaceSpec) (Namespace, error) {
	if err := validateNamespace(spec.Name); err != nil {
		return Namespace{}, err
	}
	if err := validateNamespaceLabels(spec.Labels); err != nil {
		return Namespace{}, err
	}

	// Creating one of these would be creating something the panel then refuses to
	// manage, and creating "kube-anything" is not a request made on purpose.
	if protected, reason := s.policy.Protected(spec.Name, spec.Labels); protected {
		return Namespace{}, fmt.Errorf("%w: %s is %s", ErrProtected, spec.Name, reason)
	}

	labels := map[string]string{}
	for key, value := range spec.Labels {
		labels[key] = value
	}
	labels[labelManagedBy] = managedByValue
	labels[labelPodSecurity] = s.podSecurity
	labels[nspolicy.LabelManaged] = "true"

	namespace, err := s.repo.CreateNamespace(ctx, NamespaceSpec{Name: spec.Name, Labels: labels})
	if err != nil {
		return Namespace{}, err
	}
	s.applyPolicy(&namespace)
	return namespace, nil
}

// DeleteNamespace removes a namespace, and everything in it.
//
// Deleting a namespace cascades: every workload, Secret, and
// PersistentVolumeClaim in it goes too, and the volumes those claims hold are
// released. So a namespace that still runs something is refused unless the caller
// says it meant it — "there were four things in there" is worth one more click,
// and it is the only warning anyone gets.
//
// That check is ADVISORY, and saying so is more useful than implying otherwise.
// The workloads are listed and then the namespace is deleted, and a workload
// created in between is deleted with it. Closing that would take an admission
// webhook holding the namespace still, which is a great deal of machinery to
// stop a race nobody in a single-operator lab is going to lose. It is a
// confirmation prompt with the count filled in, not a lock.
//
// The delete does carry a precondition on the namespace's UID, which closes the
// other half: the object deleted is the object that was read and checked, so a
// namespace deleted and recreated under the same name between the two is refused
// rather than removed. That one is worth closing because it is the protection
// check being raced, not a workload count.
//
// Protection is checked against the namespace as it is now rather than against
// configuration alone, because the label is half of the policy and only the live
// object carries it.
func (s *Service) DeleteNamespace(ctx context.Context, name string, force bool) error {
	if err := validateNamespace(name); err != nil {
		return err
	}

	namespace, err := s.repo.ReadNamespace(ctx, name)
	if err != nil {
		return err
	}
	if protected, reason := s.policy.Protected(namespace.Name, namespace.Labels); protected {
		return fmt.Errorf("%w: %s is %s", ErrProtected, name, reason)
	}

	if !force {
		workloads, err := s.repo.ListWorkloads(ctx, name)
		if err != nil {
			return err
		}
		if len(workloads) > 0 {
			return fmt.Errorf("%w: %s still runs %d workloads", ErrNotEmpty, name, len(workloads))
		}
	}

	return s.repo.DeleteNamespace(ctx, name, namespace.UID)
}

// applyPolicy decorates a namespace with what the panel may do to it.
//
// Decided here rather than in the repository because it is a rule rather than a
// fact about the cluster, and rules live in the service.
func (s *Service) applyPolicy(namespace *Namespace) {
	namespace.Protected, namespace.ProtectedReason =
		s.policy.Protected(namespace.Name, namespace.Labels)
	namespace.HelmManaged = s.policy.Managed(namespace.Name, namespace.Labels)
}

// validateNamespaceLabels refuses the label keys a caller may not set.
func validateNamespaceLabels(labels map[string]string) error {
	for key := range labels {
		if key == nspolicy.LabelProtected {
			return fmt.Errorf("%w: %s is set by this panel, not by a request",
				ErrInvalidName, nspolicy.LabelProtected)
		}

		prefix, _, hasPrefix := strings.Cut(key, "/")
		if !hasPrefix {
			continue
		}
		for _, reserved := range reservedLabelSuffixes {
			if prefix == reserved || strings.HasSuffix(prefix, "."+reserved) {
				return fmt.Errorf("%w: labels under %s are reserved and may not be set here",
					ErrInvalidName, reserved)
			}
		}
	}
	return nil
}
