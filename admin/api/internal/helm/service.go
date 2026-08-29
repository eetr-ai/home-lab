package helm

import (
	"context"
	"fmt"
	"log/slog"
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

// Self identifies the release this process is running from.
//
// Read from the downward API and the chart, so it is right whatever the release
// was named. Empty when either is unset, which simply means no operation is ever
// recognised as a self-upgrade — the safe direction, because the only thing that
// recognition does is stop Helm waiting.
type Self struct {
	Namespace string
	Release   string
}

// Matches reports whether an operation targets the release this process is
// running from.
func (s Self) Matches(namespace, release string) bool {
	return s.Namespace != "" && s.Release != "" &&
		s.Namespace == namespace && s.Release == release
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
	store   DeploymentStore
	policy  nspolicy.Policy
	self    Self
	locks   *locks
	timeout time.Duration
	logger  *slog.Logger
}

// NewService builds the service.
func NewService(repo repository, deployments DeploymentStore, policy nspolicy.Policy,
	self Self, timeout time.Duration, logger *slog.Logger,
) *Service {
	return &Service{
		repo:    repo,
		store:   deployments,
		policy:  policy,
		self:    self,
		locks:   newLocks(),
		timeout: timeout,
		logger:  logger,
	}
}

// ListReleases returns every release this lab can see.
//
// When the policy names the namespaces, those are enumerated. When it manages
// every unprotected namespace, the empty list asks Helm to look cluster-wide,
// which the cluster-scoped grant that mode renders makes possible.
func (s *Service) ListReleases(ctx context.Context) ([]Release, error) {
	if s.policy.ManagesEverything() {
		return s.repo.ListReleases(ctx, nil)
	}

	namespaces := s.policy.ManagedNamespaces()
	if len(namespaces) == 0 {
		return nil, ErrNotConfigured
	}
	return s.repo.ListReleases(ctx, namespaces)
}

// ListNamespaceReleases returns the releases in one managed namespace.
func (s *Service) ListNamespaceReleases(ctx context.Context, namespace string) ([]Release, error) {
	if err := s.checkNamespace(namespace); err != nil {
		return nil, err
	}
	return s.repo.ListReleases(ctx, []string{namespace})
}

// ReadRelease returns one release with the values it was configured with.
func (s *Service) ReadRelease(ctx context.Context, namespace, name string) (ReleaseDetail, error) {
	if err := s.checkRelease(namespace, name); err != nil {
		return ReleaseDetail{}, err
	}
	return s.repo.ReadRelease(ctx, namespace, name)
}

// ReadHistory returns a release's revisions, newest first.
func (s *Service) ReadHistory(ctx context.Context, namespace, name string) ([]Revision, error) {
	if err := s.checkRelease(namespace, name); err != nil {
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
func (s *Service) checkRelease(namespace, name string) error {
	if err := s.checkNamespace(namespace); err != nil {
		return err
	}
	return validateReleaseName(name)
}

// checkNamespace refuses a namespace this slice may not touch, and says which
// kind of refusal it is.
//
// Protected and unmanaged are distinguished because they mean different things to
// whoever asked. Protected is permanent — platform-system will never be a Helm
// target from here. Unmanaged is a configuration decision that can be changed,
// and telling an operator which one they have hit is the difference between
// editing a values file and giving up.
//
// The label half of the policy is not checked here: reading it needs the live
// namespace, and this slice deliberately has no cluster read of its own. The
// grant is what actually decides which Secrets are reachable, and this mirrors it.
func (s *Service) checkNamespace(namespace string) error {
	if err := validateNamespace(namespace); err != nil {
		return err
	}

	// DeployBlocked, not Protected: the panel's own namespace is undeletable and
	// deployable, because upgrading the panel from a pipeline is the reason this
	// feature exists.
	if blocked, reason := s.policy.DeployBlocked(namespace, nil); blocked {
		return fmt.Errorf("%w: %s is %s", ErrProtected, namespace, reason)
	}

	if s.policy.ManagesEverything() {
		return nil
	}

	for _, managed := range s.policy.ManagedNamespaces() {
		if managed == namespace {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrUnmanaged, namespace)
}
