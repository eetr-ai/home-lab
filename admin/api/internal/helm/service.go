package helm

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"time"

	"github.com/eetr-ai/home-lab/admin/api/internal/nspolicy"
)

// repository is the Helm storage and the chart repositories this service needs.
// Declared here, where it is consumed, so the service can be tested without a
// cluster or a registry.
type repository interface {
	ListReleases(ctx context.Context, namespaces []string) ([]Release, error)
	ReadRelease(ctx context.Context, namespace, name string) (ReleaseDetail, error)
	ReadHistory(ctx context.Context, namespace, name string) ([]Revision, error)
	ListChartVersions(ctx context.Context, source ChartSource) ([]ChartVersion, error)
	Install(ctx context.Context, spec installSpec) (Release, error)
	Upgrade(ctx context.Context, spec upgradeSpec) (Release, error)
	Rollback(ctx context.Context, namespace, name string, revision int) error
	Uninstall(ctx context.Context, namespace, name string) error
}

// Jobs is how one Helm operation is started and followed. Declared here, where it
// is consumed, so the service can be tested without a cluster.
//
// Exported, like DeploymentStore and for the same reason: it is optional, and the
// caller has to be able to declare a nil one. Handing a nil *JobRepository to a
// parameter of interface type produces an interface that is not nil, and the
// service's "can this lab deploy?" check would then be wrong in the one direction
// that matters — it would try to.
type Jobs interface {
	CreateJob(ctx context.Context, spec JobSpec, ref ReleaseRef, actor string) (Job, error)
	ReadJob(ctx context.Context, name string) (Job, error)
	ListJobs(ctx context.Context, filter JobFilter) ([]Job, error)
	WatchJob(ctx context.Context, name string) (<-chan Job, error)
	PodLogs(ctx context.Context, pod string, follow bool, tail int64) (io.ReadCloser, error)
}

// DeploymentStore is the record of what this lab has declared. Declared here,
// where it is consumed, so the service can be tested without a database.
//
// A second interface beside repository rather than one merged with it, because
// the two answer different questions: repository says what the cluster is doing,
// DeploymentStore says what was asked for. Keeping them apart is what makes it
// obvious, at every call site, which of the two a piece of code is trusting.
//
// Exported, unlike repository, for one specific reason: it is optional, and the
// caller has to be able to declare a nil one. Handing a nil *Store to a
// parameter of interface type produces an interface that is not nil, and the
// service's "was a store configured?" check would then be wrong in the one
// direction that matters — it would try to use it.
type DeploymentStore interface {
	ListDeployments(ctx context.Context, namespace string) ([]Deployment, error)
	ReadDeployment(ctx context.Context, id string) (Deployment, error)
	CreateDeployment(ctx context.Context, deployment Deployment,
		first DeploymentVersion) (Deployment, error)
	DeleteDeployment(ctx context.Context, id string) error
	ListVersions(ctx context.Context, id string) ([]DeploymentVersion, error)
	ReadVersion(ctx context.Context, id string, number int) (DeploymentVersion, error)
	AppendVersion(ctx context.Context, id string,
		version DeploymentVersion) (DeploymentVersion, error)
	MarkRolledOut(ctx context.Context, id string, number, helmRevision int) error
}

// Enrolment answers which namespaces this slice may work in.
//
// Declared here, where it is consumed. It replaces a list read from an
// environment variable, and the difference is the point: enrolling a namespace
// used to mean reinstalling the chart and restarting these pods, so the answer
// could not change while the process ran. Now it is read from the cluster, and a
// namespace enrolled a second ago is deployable without anything being restarted.
type Enrolment interface {
	Managed(ctx context.Context) ([]string, error)
}

// Service manages the Helm releases in the namespaces this lab manages.
//
// What it will not do is decided here rather than left for each endpoint to
// remember: a namespace this lab has not made a Helm target is refused before
// anything is read or written, so the set of namespaces this slice ever touches
// is the permitted one and nothing else. That is what bounds the Secret access
// the whole feature rests on.
//
// What a release *is* comes from Helm's own storage — its status, its revisions,
// its rendered notes are Secrets in the namespace it was installed into. Nothing
// about cluster reality is cached or inferred anywhere else, which is what lets
// both API replicas agree without coordinating and what makes a release installed
// by hand visible here without anything having recorded it.
type Service struct {
	repo repository
	// store is nil when this lab configured no database, and every deployment
	// route then answers 501. The release routes still work: reading the cluster
	// never needed a record.
	store DeploymentStore
	// jobs performs every mutation. Nothing writes to the cluster from this
	// process any more.
	jobs Jobs
	// enrol is nil when no namespace can be enrolled at all, and every route that
	// needs one then answers 501.
	enrol   Enrolment
	policy  nspolicy.Policy
	timeout time.Duration
	logger  *slog.Logger
}

// NewService builds the service.
func NewService(repo repository, deployments DeploymentStore, runner Jobs, enrol Enrolment,
	policy nspolicy.Policy, timeout time.Duration, logger *slog.Logger,
) *Service {
	return &Service{
		repo:    repo,
		store:   deployments,
		jobs:    runner,
		enrol:   enrol,
		policy:  policy,
		timeout: timeout,
		logger:  logger,
	}
}

// ListReleases returns every release this lab can see.
//
// The namespaces are enumerated rather than looked for cluster-wide, and that is
// deliberate: reading a release means reading Secrets, and the panel holds a
// grant only in the namespaces it enrolled. Asking Helm to search everywhere
// would be asking for a 403 from every namespace it is not permitted in.
func (s *Service) ListReleases(ctx context.Context) ([]Release, error) {
	namespaces, err := s.managed(ctx)
	if err != nil {
		return nil, err
	}
	return s.repo.ListReleases(ctx, namespaces)
}

// managed returns the namespaces this slice may work in, and refuses when there
// are none — which is the honest reply for a capability that was built and has
// not been switched on anywhere.
func (s *Service) managed(ctx context.Context) ([]string, error) {
	if s.enrol == nil {
		return nil, ErrNotConfigured
	}
	namespaces, err := s.enrol.Managed(ctx)
	if err != nil {
		return nil, err
	}
	if len(namespaces) == 0 {
		return nil, ErrNotConfigured
	}
	return namespaces, nil
}

// ListNamespaceReleases returns the releases in one managed namespace.
func (s *Service) ListNamespaceReleases(ctx context.Context, namespace string) ([]Release, error) {
	if err := s.checkNamespace(ctx, namespace); err != nil {
		return nil, err
	}
	return s.repo.ListReleases(ctx, []string{namespace})
}

// ReadRelease returns one release with the values it was configured with.
func (s *Service) ReadRelease(ctx context.Context, namespace, name string) (ReleaseDetail, error) {
	if err := s.checkRelease(ctx, namespace, name); err != nil {
		return ReleaseDetail{}, err
	}
	return s.repo.ReadRelease(ctx, namespace, name)
}

// ReadHistory returns a release's revisions, newest first.
func (s *Service) ReadHistory(ctx context.Context, namespace, name string) ([]Revision, error) {
	if err := s.checkRelease(ctx, namespace, name); err != nil {
		return nil, err
	}
	return s.repo.ReadHistory(ctx, namespace, name)
}

// ListChartVersions returns the versions a chart reference offers.
//
// This is what fills the version picker, and it is the only place the API reaches
// out to a registry on a read. A reference that cannot be parsed is a 400 before
// anything is fetched.
func (s *Service) ListChartVersions(ctx context.Context, reference string) ([]ChartVersion, error) {
	source, err := ParseChartRef(reference)
	if err != nil {
		return nil, err
	}
	return s.repo.ListChartVersions(ctx, source)
}

// checkRelease refuses a namespace this slice may not touch and a release name
// Helm would not accept.
func (s *Service) checkRelease(ctx context.Context, namespace, name string) error {
	if err := s.checkNamespace(ctx, namespace); err != nil {
		return err
	}
	return validateReleaseName(name)
}

// checkNamespace refuses a namespace this slice may not touch, and says which
// kind of refusal it is.
//
// Protected and unmanaged are distinguished because they mean different things to
// whoever asked. Protected is permanent — platform-system will never be a Helm
// target from here. Unmanaged can be changed, and now it can be changed from the
// panel: enrolling the namespace is a button rather than a chart release, which
// is the whole reason this reads the cluster instead of an environment variable.
//
// The enrolled set is the authority, not the label alone. A namespace can carry
// the label and have no role bindings — which is what a half-finished enrolment
// looks like — and permitting a deploy there would produce a 403 out of the API
// server instead of a sentence naming the missing thing.
func (s *Service) checkNamespace(ctx context.Context, namespace string) error {
	if err := validateNamespace(namespace); err != nil {
		return err
	}

	// DeployBlocked, not Protected: the panel's own namespace is undeletable and
	// deployable, because upgrading the panel from a pipeline is the reason this
	// feature exists.
	if blocked, reason := s.policy.DeployBlocked(namespace, nil); blocked {
		return fmt.Errorf("%w: %s is %s", ErrProtected, namespace, reason)
	}

	managed, err := s.managed(ctx)
	if err != nil {
		return err
	}
	if slices.Contains(managed, namespace) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrUnmanaged, namespace)
}
