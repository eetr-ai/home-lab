package kube

import "context"

// repository is the cluster access this service needs. Declared here, where it is
// consumed, so the service can be tested without a cluster.
type repository interface {
	ListNamespaces(ctx context.Context) ([]Namespace, error)
	ListWorkloads(ctx context.Context, namespace string) ([]Workload, error)
	ListPods(ctx context.Context, namespace string) ([]Pod, error)
	ListEvents(ctx context.Context, namespace string) ([]Event, error)
}

// Service reads the cluster.
//
// Read-only, and deliberately so. The panel exists to say what is running and why
// something is not; changing a workload is what the repository's Helm releases and
// `kubectl` are for, and an API that could scale or delete would need a much more
// careful answer to "who is allowed to" than "whoever can sign in".
type Service struct {
	repo repository
}

// NewService builds the service.
func NewService(repo repository) *Service {
	return &Service{repo: repo}
}

// ListNamespaces returns every namespace in the cluster.
func (s *Service) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	return s.repo.ListNamespaces(ctx)
}

// ListWorkloads returns the Deployments, StatefulSets, and DaemonSets in one
// namespace, as a single list.
func (s *Service) ListWorkloads(ctx context.Context, namespace string) ([]Workload, error) {
	if err := validateNamespace(namespace); err != nil {
		return nil, err
	}
	return s.repo.ListWorkloads(ctx, namespace)
}

// ListPods returns the pods in one namespace.
func (s *Service) ListPods(ctx context.Context, namespace string) ([]Pod, error) {
	if err := validateNamespace(namespace); err != nil {
		return nil, err
	}
	return s.repo.ListPods(ctx, namespace)
}

// ListEvents returns the recent events in one namespace, most recent first.
//
// Events are where the reason for a stuck pod lives — the pod says
// ImagePullBackOff, the event says which registry refused and why.
func (s *Service) ListEvents(ctx context.Context, namespace string) ([]Event, error) {
	if err := validateNamespace(namespace); err != nil {
		return nil, err
	}
	return s.repo.ListEvents(ctx, namespace)
}
