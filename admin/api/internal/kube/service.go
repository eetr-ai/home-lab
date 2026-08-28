package kube

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/eetr-ai/home-lab/admin/api/internal/nspolicy"
)

// repository is the cluster access this service needs. Declared here, where it is
// consumed, so the service can be tested without a cluster.
type repository interface {
	ListNamespaces(ctx context.Context) ([]Namespace, error)
	ReadNamespace(ctx context.Context, name string) (Namespace, error)
	CreateNamespace(ctx context.Context, spec NamespaceSpec) (Namespace, error)
	DeleteNamespace(ctx context.Context, name, uid string) error
	ListWorkloads(ctx context.Context, namespace string) ([]Workload, error)
	ListPods(ctx context.Context, namespace string) ([]Pod, error)
	ListEvents(ctx context.Context, namespace string) ([]Event, error)
	ListNodes(ctx context.Context) ([]Node, error)
	ReadStorage(ctx context.Context) (Storage, error)
	ReadSummary(ctx context.Context) (Summary, error)
	ReadWorkload(ctx context.Context, kind, namespace, name string) (WorkloadDetail, error)
	PodLogs(ctx context.Context, namespace, pod string, options LogOptions) (io.ReadCloser, error)
	RestartWorkload(ctx context.Context, kind, namespace, name string, at time.Time) error
	ScaleWorkload(ctx context.Context, kind, namespace, name string, replicas int32) error
}

// Service reads the cluster, and rolls or resizes what is already on it.
//
// Reads are the bulk of it. The two writes — a rollout restart and a replica
// count — are deliberately the only ones: both are reversible, both are things
// that already happen without the panel, and neither can bring something into
// existence or take it away. What a workload *is* still comes from this
// repository's Helm releases; this changes only how many of it there are and
// when it last started.
//
// Note that the API itself does not decide who may do this. Every verified caller
// can, the same as for the database slices; the panel gates writes on
// ADMIN_WRITE_EMAILS in its own layer. That is a real limit and worth knowing
// about before handing a token to anything.
type Service struct {
	repo        repository
	policy      nspolicy.Policy
	podSecurity string
}

// NewService builds the service, and refuses a Pod Security level Kubernetes
// would not accept.
//
// The policy is what decides which namespaces may not be deleted. It is passed in
// rather than read here because it is the same policy the Helm slice enforces, and
// two slices deciding protection separately is two answers to one question.
//
// podSecurity is the Pod Security admission level stamped on a namespace this
// panel creates. Empty means baseline, which is the level that stops the things
// nobody wants — host mounts, privileged containers, host networking — while
// still running ordinary charts.
//
// It is checked here rather than trusted, and returning an error rather than
// falling back is the point. Kubernetes accepts exactly three values on that
// label; anything else is refused by the API server, so a typo like "basline"
// would leave every namespace creation failing with a message about a label
// nobody typed. Failing at startup turns that into one clear line in the log
// before the process serves anything. The chart's schema constrains this too,
// and that is not enough on its own: the value also arrives from an environment
// variable, and a rule that holds only when the chart wrote it is not a rule.
func NewService(repo repository, policy nspolicy.Policy, podSecurity string) (*Service, error) {
	if podSecurity == "" {
		podSecurity = defaultPodSecurity
	}
	if !slices.Contains(podSecurityLevels, podSecurity) {
		return nil, fmt.Errorf(
			"%w: pod security level %q is not one of %s",
			ErrInvalidName, podSecurity, strings.Join(podSecurityLevels, ", "))
	}
	return &Service{repo: repo, policy: policy, podSecurity: podSecurity}, nil
}

// ListNamespaces returns every namespace in the cluster, each carrying whether
// the panel may delete it.
//
// Every namespace, including the protected ones: protection is about writing, and
// hiding platform-system from the list would take away the reading this panel
// exists for.
func (s *Service) ListNamespaces(ctx context.Context) ([]Namespace, error) {
	namespaces, err := s.repo.ListNamespaces(ctx)
	if err != nil {
		return nil, err
	}
	for i := range namespaces {
		s.applyPolicy(&namespaces[i])
	}
	return namespaces, nil
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

// ListNodes returns every machine in the cluster, with what is scheduled against
// it and what is actually being used.
//
// Usage arrives only when metrics-server is installed and has collected a sample.
// Its absence is reported as a missing reading rather than as a failure: a
// cluster without it is a normal cluster, and the capacity figures beside it are
// worth showing on their own.
func (s *Service) ListNodes(ctx context.Context) ([]Node, error) {
	return s.repo.ListNodes(ctx)
}

// ReadStorage returns the cluster's persistent volume claims and volumes.
func (s *Service) ReadStorage(ctx context.Context) (Storage, error) {
	return s.repo.ReadStorage(ctx)
}

// ReadSummary returns the rollup the overview dashboard renders.
//
// One call rather than five, because a dashboard that issues a request per tile
// renders in pieces and fails in pieces.
func (s *Service) ReadSummary(ctx context.Context) (Summary, error) {
	return s.repo.ReadSummary(ctx)
}

// ReadWorkload returns one workload with its pods, services, claims, and events.
func (s *Service) ReadWorkload(
	ctx context.Context, kind, namespace, name string,
) (WorkloadDetail, error) {
	if err := validateTarget(kind, namespace, name); err != nil {
		return WorkloadDetail{}, err
	}
	return s.repo.ReadWorkload(ctx, kind, namespace, name)
}

// PodLogs opens a pod's log as a stream. The caller must close the reader.
//
// The tail is capped rather than trusted: a caller asking for every line of a pod
// that has been up for weeks would hold the connection for as long as it takes to
// send them, which is not a request anybody makes on purpose.
func (s *Service) PodLogs(
	ctx context.Context, namespace, pod string, options LogOptions,
) (io.ReadCloser, error) {
	if err := validateNamespace(namespace); err != nil {
		return nil, err
	}
	if err := validateName(pod, "pod"); err != nil {
		return nil, err
	}
	// A container name is a DNS label too, and it goes into a query parameter
	// rather than a path — but an unvalidated one still reaches the API server.
	if options.Container != "" {
		if err := validateLabelName(options.Container, "container"); err != nil {
			return nil, err
		}
	}

	if options.Tail <= 0 || options.Tail > maxLogTail {
		options.Tail = defaultLogTail
	}
	return s.repo.PodLogs(ctx, namespace, pod, options)
}

// RestartWorkload rolls a workload's pods.
func (s *Service) RestartWorkload(ctx context.Context, kind, namespace, name string) error {
	if err := validateTarget(kind, namespace, name); err != nil {
		return err
	}
	if err := validateActionable(kind); err != nil {
		return err
	}
	return s.repo.RestartWorkload(ctx, kind, namespace, name, time.Now())
}

// ScaleWorkload sets a workload's replica count.
func (s *Service) ScaleWorkload(
	ctx context.Context, kind, namespace, name string, replicas int32,
) error {
	if err := validateTarget(kind, namespace, name); err != nil {
		return err
	}
	if err := validateActionable(kind); err != nil {
		return err
	}
	if replicas < 0 || replicas > maxReplicas {
		return fmt.Errorf("%w: replicas must be between 0 and %d", ErrInvalidName, maxReplicas)
	}
	return s.repo.ScaleWorkload(ctx, kind, namespace, name, replicas)
}

// validateActionable rejects a kind the panel reads but does not change.
//
// A DaemonSet's size is however many nodes it matches rather than a number
// anything can set, so there is no scale subresource to write. Restarting one
// would be an ordinary patch, but it is withheld for the same reason: a section
// that can restart what it cannot resize is a confusing half-capability. Add it
// in the change that has a DaemonSet here worth restarting.
func validateActionable(kind string) error {
	if kind == KindDaemonSet {
		return fmt.Errorf("%w: a %s is sized by the nodes it matches", ErrUnsupportedKind, kind)
	}
	return nil
}

// validateTarget checks the three parameters every workload operation takes.
func validateTarget(kind, namespace, name string) error {
	if err := validateKind(kind); err != nil {
		return err
	}
	if err := validateNamespace(namespace); err != nil {
		return err
	}
	return validateName(name, "workload")
}
