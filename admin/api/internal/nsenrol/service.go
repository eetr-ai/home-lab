package nsenrol

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/eetr-ai/home-lab/admin/api/internal/nspolicy"
)

// ErrProtected reports a namespace that may not be enrolled. Protection wins over
// enrolment for the same reason it wins over everything else: a namespace holding
// credentials that matter is not one to make readable by pressing a button.
var ErrProtected = errors.New("protected namespace")

// repository is the cluster access this needs, declared here where it is consumed
// so the service can be tested without a cluster.
type repository interface {
	ListBindings(ctx context.Context, namespace string, names []string) ([]Binding, error)
	ListAllBindings(ctx context.Context, names []string) (map[string][]Binding, error)
	CreateBinding(ctx context.Context, namespace string, binding Binding) error
	DeleteBinding(ctx context.Context, namespace, name string) error
	ListCandidates(ctx context.Context) ([]Candidate, error)
}

// Service enrols namespaces, and answers which ones are enrolled.
type Service struct {
	repo   repository
	config Config
	policy nspolicy.Policy
}

// NewService builds the service.
func NewService(repo repository, config Config, policy nspolicy.Policy) *Service {
	return &Service{repo: repo, config: config, policy: policy}
}

// State reports how completely a namespace is enrolled.
//
// The labels are the caller's — it has just read the namespace — because this is
// asked once per namespace in a listing and re-reading each one would turn a
// listing into a request per row.
func (s *Service) State(ctx context.Context, namespace string, labels map[string]string) (State, error) {
	if !s.policy.Managed(namespace, labels) {
		// Not a candidate at all. Reporting "missing" here would invite a repair
		// button on a namespace that is not asking to be one.
		return StateMissing, nil
	}

	live, err := s.repo.ListBindings(ctx, namespace, s.config.Names())
	if err != nil {
		return "", err
	}
	return s.config.Decide(live).State, nil
}

// States reports enrolment for a whole listing, in one request.
//
// The labels come from the namespaces the caller already read, and the bindings
// arrive grouped from a single cluster-wide list — so showing enrolment on a page
// of namespaces costs one API call rather than one per row.
//
// A namespace that has not asked to be a Helm target is left out of the answer
// entirely rather than reported as missing, which is what stops the panel
// offering to repair something nobody wanted.
func (s *Service) States(ctx context.Context, namespaces []Candidate) (map[string]State, error) {
	grouped, err := s.repo.ListAllBindings(ctx, s.config.Names())
	if err != nil {
		return nil, err
	}

	states := make(map[string]State, len(namespaces))
	for _, namespace := range namespaces {
		if !s.policy.Managed(namespace.Name, namespace.Labels) {
			continue
		}
		states[namespace.Name] = s.config.Decide(grouped[namespace.Name]).State
	}
	return states, nil
}

// Reconcile makes a namespace's enrolment match what enrolment means, and reports
// the state it ended in.
//
// Idempotent on purpose: this is both "set it up" and "repair it", because they
// are the same request. A namespace that is already correct is not written to.
//
// Deletes come before creates, and only for bindings that are present and wrong.
// roleRef is immutable, so the wrong ones cannot be edited into shape — which is
// exactly the case a lab upgraded from an older chart is in, and exactly the case
// nothing else would ever fix.
func (s *Service) Reconcile(ctx context.Context, namespace string, labels map[string]string) (State, error) {
	if blocked, reason := s.policy.DeployBlocked(namespace, labels); blocked {
		return "", fmt.Errorf("%w: %s is %s", ErrProtected, namespace, reason)
	}

	live, err := s.repo.ListBindings(ctx, namespace, s.config.Names())
	if err != nil {
		return "", err
	}

	plan := s.config.Decide(live)
	if plan.Done() {
		return StateEnrolled, nil
	}

	for _, name := range plan.Replace {
		if err := s.repo.DeleteBinding(ctx, namespace, name); err != nil {
			return "", err
		}
	}
	for _, binding := range plan.Create {
		if err := s.repo.CreateBinding(ctx, namespace, binding); err != nil {
			return "", err
		}
	}
	return StateEnrolled, nil
}

// Revoke removes the panel's bindings from a namespace.
//
// The label is left alone, and that is deliberate: this package writes
// RoleBindings and the namespace's labels belong to whoever owns the namespace.
// Nothing works without the bindings, so removing them is the whole of revoking.
func (s *Service) Revoke(ctx context.Context, namespace string) error {
	for _, name := range s.config.Names() {
		if err := s.repo.DeleteBinding(ctx, namespace, name); err != nil {
			return err
		}
	}
	return nil
}

// Managed returns the namespaces Helm may work in, sorted.
//
// A namespace qualifies when it says it is a target, policy agrees, and the
// bindings are actually there. The last condition is what stops this from
// answering with namespaces the panel would then fail to read: enumerating a
// namespace's releases means reading its Secrets, and a namespace with no binding
// is one where that is a 403 rather than an empty list.
func (s *Service) Managed(ctx context.Context) ([]string, error) {
	candidates, err := s.repo.ListCandidates(ctx)
	if err != nil {
		return nil, err
	}

	// One binding list for all of them rather than one per candidate. This is on
	// the path of every Helm read, so a request per namespace would be a request
	// per namespace on every page of the section.
	grouped, err := s.repo.ListAllBindings(ctx, s.config.Names())
	if err != nil {
		return nil, err
	}

	managed := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if !s.policy.Managed(candidate.Name, candidate.Labels) {
			continue
		}
		if s.config.Decide(grouped[candidate.Name]).State != StateEnrolled {
			continue
		}
		managed = append(managed, candidate.Name)
	}
	sort.Strings(managed)
	return managed, nil
}
