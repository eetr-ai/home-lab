package helm

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/eetr-ai/home-lab/admin/api/internal/nspolicy"
)

// repository is the release storage this service needs. Declared here, where it
// is consumed, so the service can be tested without a cluster or a chart.
type repository interface {
	ListReleases(ctx context.Context, namespaces []string) ([]Release, error)
	ReadRelease(ctx context.Context, namespace, name string) (ReleaseDetail, error)
	ReadHistory(ctx context.Context, namespace, name string) ([]Revision, error)
	ListChartVersions(ctx context.Context, source ChartSource) ([]ChartVersion, error)
	Install(ctx context.Context, req InstallRequest, source ChartSource) (Release, error)
	Upgrade(ctx context.Context, req UpgradeRequest, source ChartSource) (Release, error)
	Rollback(ctx context.Context, namespace, name string, revision int) error
	Uninstall(ctx context.Context, namespace, name string) error
}

// Service manages the Helm releases in the namespaces this lab has named.
//
// Two rules bound everything it does, and both are checked here rather than left
// for each endpoint to remember. A namespace this lab has not made a Helm target
// is refused before anything is read or written, so the set of namespaces this
// slice ever touches is the configured one and nothing else. And a chart that is
// not in the catalog cannot be installed at all — the catalog is the allowlist
// that turns "install an arbitrary chart", which is cluster-admin, into "install
// something somebody vetted".
//
// There is no database behind any of this. A release's identity, values, and
// history are Secrets in the namespace it was installed into, which is what lets
// both API replicas agree without coordinating, and what makes a release
// installed by hand visible here without anything having recorded it.
type Service struct {
	repo     repository
	policy   nspolicy.Policy
	catalog  Catalog
	versions *versionCache
	locks    *locks
	timeout  time.Duration
	logger   *slog.Logger
}

// NewService builds the service.
// timeout bounds one install, upgrade, rollback, or uninstall. It is off the
// request path entirely, so it is generous: what it protects against is an
// operation that never finishes leaving a release pending forever.
func NewService(repo repository, policy nspolicy.Policy, catalog Catalog,
	timeout time.Duration, logger *slog.Logger,
) *Service {
	return &Service{
		repo:     repo,
		policy:   policy,
		catalog:  catalog,
		versions: newVersionCache(),
		locks:    newLocks(),
		timeout:  timeout,
		logger:   logger,
	}
}

// ListReleases returns every release in every managed namespace.
//
// The configured list is what is enumerated. Finding managed namespaces by
// reading the cluster and checking labels would need a cluster-wide grant on
// Helm's release Secrets, which is the one thing this design refuses to hold.
func (s *Service) ListReleases(ctx context.Context) ([]Release, error) {
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
	if err := s.checkNamespace(namespace); err != nil {
		return ReleaseDetail{}, err
	}
	if err := validateReleaseName(name); err != nil {
		return ReleaseDetail{}, err
	}
	return s.repo.ReadRelease(ctx, namespace, name)
}

// ReadHistory returns a release's revisions, newest first.
func (s *Service) ReadHistory(ctx context.Context, namespace, name string) ([]Revision, error) {
	if err := s.checkNamespace(namespace); err != nil {
		return nil, err
	}
	if err := validateReleaseName(name); err != nil {
		return nil, err
	}
	return s.repo.ReadHistory(ctx, namespace, name)
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
// configured list is the half that decides which Secrets are reachable at all,
// because it is what renders the Role.
func (s *Service) checkNamespace(namespace string) error {
	if err := validateNamespace(namespace); err != nil {
		return err
	}

	if protected, reason := s.policy.Protected(namespace, nil); protected {
		return fmt.Errorf("%w: %s is %s", ErrProtected, namespace, reason)
	}

	for _, managed := range s.policy.ManagedNamespaces() {
		if managed == namespace {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrUnmanaged, namespace)
}
