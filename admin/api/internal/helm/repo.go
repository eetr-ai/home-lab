package helm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"helm.sh/helm/v4/pkg/action"
	chartapi "helm.sh/helm/v4/pkg/chart"
	chartv2 "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/release"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	repo "helm.sh/helm/v4/pkg/repo/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
)

// Repository reads Helm's release storage.
//
// Note what these methods do not do with their context: Helm's actions take
// none. A request the caller abandoned still runs to completion here, which for a
// read is a wasted listing and nothing worse. The context stays on the signatures
// because the interface the service declares is this package's contract rather
// than Helm's, and the write operations that come later do have something to
// bound.
//
// This is the only file in the slice that imports helm.sh/helm — everything above
// it works in this package's own types. That is deliberate and it is the same rule
// the cluster slice follows with corev1: it keeps a Helm minor upgrade from
// changing the panel's wire format, and it keeps the service testable without a
// cluster or a chart.
type Repository struct {
	clients *clients
	logger  *slog.Logger
	// timeout bounds one install, upgrade, rollback, or uninstall. It is off the
	// request path entirely — the caller was answered long before — so it is
	// generous, and what it protects against is an operation that never finishes
	// holding a release in a pending state forever.
	timeout time.Duration
}

// NewRepository builds the repository. It contacts nothing.
func NewRepository(logger *slog.Logger, timeout time.Duration) (*Repository, error) {
	clients, err := newClients(logger)
	if err != nil {
		return nil, err
	}
	return &Repository{clients: clients, logger: logger, timeout: timeout}, nil
}

// ListReleases returns the releases in each of the given namespaces, or every
// release on the cluster when no namespace is named.
//
// One listing per namespace by default, rather than one cluster-wide listing,
// because a cluster-wide one needs a cluster-wide grant on Secrets — and Helm
// keeps a release in a Secret. Asking each namespace separately is what lets the
// grant be a Role in that namespace and nothing more.
//
// An empty list means the lab chose the cluster-scoped grant instead, so the
// cluster-wide listing is available and is what the caller wants. It is not the
// same as "no namespaces": the service reports that case as unconfigured before
// reaching here, because listing everything for a lab that named nothing would
// be the opposite of what it asked for.
//
// A namespace that cannot be read is logged and skipped rather than failing the
// whole request. The usual cause is a namespace named in configuration that does
// not exist yet, or one whose Role has not been applied, and answering "nothing
// at all" for every other namespace because of it would be the wrong trade.
func (r *Repository) ListReleases(ctx context.Context, namespaces []string) ([]Release, error) {
	releases := []Release{}

	if len(namespaces) == 0 {
		found, err := r.listEverywhere()
		if err != nil {
			return nil, err
		}
		return sortReleases(found), nil
	}

	for _, namespace := range namespaces {
		found, err := r.listNamespace(namespace)
		if err != nil {
			r.logger.Warn("could not list helm releases",
				slog.String("namespace", namespace), slog.Any("error", err))
			continue
		}
		releases = append(releases, found...)
	}

	return sortReleases(releases), nil
}

func sortReleases(releases []Release) []Release {
	sort.Slice(releases, func(a, b int) bool {
		if releases[a].Namespace != releases[b].Namespace {
			return releases[a].Namespace < releases[b].Namespace
		}
		return releases[a].Name < releases[b].Name
	})
	return releases
}

// listEverywhere lists releases in every namespace at once.
//
// Only reachable when the lab rendered the cluster-scoped grant.
//
// The configuration is built with an empty namespace, not the pod's. Setting
// AllNamespaces on the action is not enough on its own: the storage driver is
// bound when the configuration is initialised, and one bound to a namespace
// keeps answering for that namespace whatever the action asks for. Passing the
// pod's namespace here returns exactly one release — the panel's own — which
// looks like a cluster with nothing on it.
func (r *Repository) listEverywhere() ([]Release, error) {
	configuration, err := r.clients.configurationFor("", forReading)
	if err != nil {
		return nil, err
	}

	list := action.NewList(configuration)
	list.AllNamespaces = true
	list.StateMask = action.ListAll

	found, err := list.Run()
	if err != nil {
		return nil, translate(err, "list releases across the cluster")
	}

	releases := make([]Release, 0, len(found))
	for _, item := range found {
		converted, err := releaseFrom(item)
		if err != nil {
			r.logger.Warn("skipping an unreadable helm release", slog.Any("error", err))
			continue
		}
		releases = append(releases, converted)
	}
	return releases, nil
}

func (r *Repository) listNamespace(namespace string) ([]Release, error) {
	configuration, err := r.clients.configurationFor(namespace, forReading)
	if err != nil {
		return nil, err
	}

	list := action.NewList(configuration)
	// Every state, not just deployed. A release stuck in pending-upgrade or left
	// failed is the one an operator most needs to see, and hiding it behind a
	// default filter would make the panel look healthy while the cluster is not.
	list.StateMask = action.ListAll

	found, err := list.Run()
	if err != nil {
		return nil, translate(err, "list releases in "+namespace)
	}

	releases := make([]Release, 0, len(found))
	for _, item := range found {
		converted, err := releaseFrom(item)
		if err != nil {
			r.logger.Warn("skipping an unreadable helm release",
				slog.String("namespace", namespace), slog.Any("error", err))
			continue
		}
		releases = append(releases, converted)
	}
	return releases, nil
}

// ReadRelease returns one release with the values it was configured with.
//
// Three calls rather than one: the release itself, the values, and nothing more
// than Helm's own client does for `helm get`. They are separate actions in the
// SDK and each reads the same stored revision, so the cost is in the Secret
// decode rather than in the round trips.
func (r *Repository) ReadRelease(ctx context.Context, namespace, name string) (ReleaseDetail, error) {
	configuration, err := r.clients.configurationFor(namespace, forReading)
	if err != nil {
		return ReleaseDetail{}, err
	}

	found, err := action.NewGet(configuration).Run(name)
	if err != nil {
		return ReleaseDetail{}, translate(err, "read release "+name)
	}

	base, err := releaseFrom(found)
	if err != nil {
		return ReleaseDetail{}, err
	}
	base.Namespace = namespace

	values, err := action.NewGetValues(configuration).Run(name)
	if err != nil {
		return ReleaseDetail{}, translate(err, "read the values of release "+name)
	}
	if values == nil {
		// A release installed with no values of its own. An empty object is the
		// honest answer and the one the panel's editor can start from; null would
		// make it show "no values" and refuse to be edited.
		values = map[string]any{}
	}

	notes := ""
	if accessor, err := release.NewAccessor(found); err == nil {
		notes = accessor.Notes()
	}

	return ReleaseDetail{Release: base, Values: values, Notes: notes}, nil
}

// ReadHistory returns a release's revisions, newest first.
func (r *Repository) ReadHistory(ctx context.Context, namespace, name string) ([]Revision, error) {
	configuration, err := r.clients.configurationFor(namespace, forReading)
	if err != nil {
		return nil, err
	}

	found, err := action.NewHistory(configuration).Run(name)
	if err != nil {
		return nil, translate(err, "read the history of release "+name)
	}

	revisions := make([]Revision, 0, len(found))
	for _, item := range found {
		converted, err := releaseFrom(item)
		if err != nil {
			r.logger.Warn("skipping an unreadable revision",
				slog.String("release", name), slog.Any("error", err))
			continue
		}
		revisions = append(revisions, Revision{
			Revision:     converted.Revision,
			Status:       converted.Status,
			ChartVersion: converted.ChartVersion,
			AppVersion:   converted.AppVersion,
			Description:  converted.Description,
			Updated:      converted.Updated,
		})
	}

	// Newest first. Helm stores them oldest first, and the revision an operator
	// wants is almost always the one that just failed.
	sort.Slice(revisions, func(a, b int) bool { return revisions[a].Revision > revisions[b].Revision })
	return revisions, nil
}

// releaseFrom translates one of Helm's releases into this slice's.
//
// It goes through release.NewAccessor rather than asserting a concrete type,
// because Helm carries more than one release shape and the accessor is the
// supported way to read any of them.
func releaseFrom(item release.Releaser) (Release, error) {
	accessor, err := release.NewAccessor(item)
	if err != nil {
		return Release{}, fmt.Errorf("read the release: %w", err)
	}

	chartName, chartVersion, appVersion := chartMetadata(accessor.Chart())

	return Release{
		Name:         accessor.Name(),
		Namespace:    accessor.Namespace(),
		Revision:     accessor.Version(),
		Status:       accessor.Status(),
		Chart:        chartName,
		ChartVersion: chartVersion,
		AppVersion:   appVersion,
		Description:  descriptionOf(item),
		Updated:      accessor.DeployedAt(),
	}, nil
}

// chartMetadata reads the three fields worth showing off a chart.
//
// The name comes from the accessor, which is typed. The two versions come from
// MetadataAsMap, because the accessor interface offers no other way to reach
// them — and that map is keyed by Helm's **Go field names**, not by the JSON
// tags on the same struct. So the keys are Version and AppVersion rather than
// the version and appVersion a reader of the chart's own YAML would expect.
//
// This is worth stating because getting it wrong is silent: a missing key yields
// an empty string, the release still loads, and the chart column is simply blank
// — which is what shipped until a real release was read back. Both spellings are
// accepted so that a change to Helm's internals surfaces as neither.
//
// All three come from the map, including the name, even though the accessor has
// a typed Name(). That method dereferences the chart's Metadata without checking
// it for nil, so a release carrying a chart without one panics the process --
// and this is reached from the Job runner as well as from a request, where a
// panic is not a failed read but a deploy that dies without saying why.
// MetadataAsMap checks, which is the whole reason to prefer it.
func chartMetadata(charter chartapi.Charter) (name, version, appVersion string) {
	accessor, err := chartapi.NewAccessor(charter)
	if err != nil {
		return "", "", ""
	}

	metadata := accessor.MetadataAsMap()
	return stringField(metadata, "Name", "name"),
		stringField(metadata, "Version", "version"),
		stringField(metadata, "AppVersion", "appVersion")
}

// stringField returns the first key that holds a string, so the caller can name
// the spelling it expects and the one it would rather not depend on.
func stringField(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := metadata[key].(string); ok && value != "" {
			return value
		}
	}
	return ""
}

// descriptionOf reads the log entry Helm writes on each revision — "Upgrade
// complete", or the reason an upgrade failed.
//
// It is deliberately not on release.Accessor, and the only release shape this
// package can name is v1: the others live in Helm's internal tree. So a release
// stored in a shape this cannot read loses its description rather than failing to
// load, which is the right direction — the description is the most useful field
// on a broken release and the least load-bearing on a healthy one.
func descriptionOf(item release.Releaser) string {
	switch typed := item.(type) {
	case *releasev1.Release:
		if typed.Info != nil {
			return typed.Info.Description
		}
	case releasev1.Release:
		if typed.Info != nil {
			return typed.Info.Description
		}
	}
	return ""
}

// translate maps Helm's and Kubernetes' errors onto this slice's.
//
// Helm reports a missing release as its own sentinel rather than as a Kubernetes
// 404, because the Secret it looked for is genuinely absent; both are mapped here
// so a handler never has to know which layer answered.
func translate(err error, what string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, driver.ErrReleaseNotFound), errors.Is(err, driver.ErrNoDeployedReleases),
		apierrors.IsNotFound(err):
		return fmt.Errorf("%w: %s", ErrNotFound, what)
	// A pod whose container has not started yet. Expected for the first moment of
	// every job, and the caller's answer is to retry rather than to give up — so
	// it must not present as an internal error.
	case strings.Contains(err.Error(), "is waiting to start"),
		strings.Contains(err.Error(), "ContainerCreating"),
		strings.Contains(err.Error(), "PodInitializing"):
		return fmt.Errorf("%w: %s", ErrNoPodYet, what)
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err),
		// Helm wraps the API server's refusal in its own message often enough
		// that the typed check alone misses it, and reading "forbidden" as a 500
		// sends an operator looking for a bug instead of a missing RoleBinding.
		strings.Contains(err.Error(), "is forbidden"):
		return fmt.Errorf("%w: %s", ErrForbidden, what)
	default:
		return fmt.Errorf("%s: %w", what, err)
	}
}

// ListChartVersions returns the versions a repository offers for one chart.
//
// Two shapes, because the two kinds of repository answer differently. An HTTP
// repository publishes index.yaml listing every chart it holds with every
// version and app version; an OCI registry answers with tags and nothing else,
// so an OCI chart reports versions with no app version rather than fetching each
// manifest to find one.
//
// Helm's downloader writes the index to its cache directory, which is why the
// pod carries a writable volume for it. Nothing else in this slice writes to
// disk.
// ListChartVersions returns the versions a chart reference offers.
//
// Neither Helm call underneath takes a context: DownloadIndexFile and
// registry.Client.Tags each use their own HTTP client and answer when they
// answer. So the lookup runs on its own goroutine and this returns as soon as
// the caller's context is done, which is what stops an unreachable registry
// holding a request open until the HTTP server stops writing at thirty seconds.
//
// The goroutine is not cancelled — there is nothing to cancel it with — so it
// runs to completion and then finds nobody listening. The channel is buffered
// for exactly that reason: an unbuffered one would leak the goroutine forever
// rather than for as long as the call takes. This is a bound on the caller's
// wait, not on the work, and it is the most that can be done without Helm
// growing context-aware getters.
func (r *Repository) ListChartVersions(ctx context.Context, source ChartSource) ([]ChartVersion, error) {
	type result struct {
		versions []ChartVersion
		err      error
	}

	done := make(chan result, 1)
	go func() {
		if source.OCI {
			versions, err := r.ociTags(source)
			done <- result{versions, err}
			return
		}
		versions, err := r.indexVersions(source)
		done <- result{versions, err}
	}()

	select {
	case answered := <-done:
		return answered.versions, answered.err
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: %s did not answer in time", ErrRepositoryUnreachable, source.URL)
	}
}

func (r *Repository) indexVersions(source ChartSource) ([]ChartVersion, error) {
	chartRepo, err := repo.NewChartRepository(
		&repo.Entry{Name: source.Chart, URL: source.URL}, getter.All(cli.New()))
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrRepositoryUnreachable, source.URL, err)
	}

	path, err := chartRepo.DownloadIndexFile()
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrRepositoryUnreachable, source.URL, err)
	}

	index, err := repo.LoadIndexFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s has an index this cannot read: %w",
			ErrRepositoryUnreachable, source.URL, err)
	}

	entries, ok := index.Entries[source.Chart]
	if !ok {
		return nil, fmt.Errorf("%w: %s does not publish a chart called %s",
			ErrNotFound, source.URL, source.Chart)
	}

	versions := make([]ChartVersion, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.Metadata == nil || entry.Removed {
			continue
		}
		versions = append(versions, ChartVersion{
			Version:    entry.Version,
			AppVersion: entry.AppVersion,
		})
	}
	return versions, nil
}

func (r *Repository) ociTags(source ChartSource) ([]ChartVersion, error) {
	reference := strings.TrimSuffix(strings.TrimPrefix(source.URL, "oci://"), "/") + "/" + source.Chart

	tags, err := r.clients.registry.Tags(reference)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrRepositoryUnreachable, source.URL, err)
	}

	versions := make([]ChartVersion, 0, len(tags))
	for _, tag := range tags {
		// An OCI registry answers with tags and nothing else. Reading an app
		// version would mean pulling every manifest, which is one request per
		// version for a field the panel shows and nothing depends on.
		versions = append(versions, ChartVersion{Version: tag})
	}
	return versions, nil
}

// maxHistory is how many revisions of a release Helm keeps.
//
// Each revision is a Secret holding the whole rendered manifest, so an unbounded
// history is unbounded Secrets in the namespace. Ten is more than anybody rolls
// back through and small enough that it does not accumulate.
const maxHistory = 10

// Install puts a new release on the cluster and waits for it to come up.
//
// The chart is located by reference and exact version. The reference was parsed
// and bounded by ParseChartRef before it reached here, which is the only thing
// between this and an arbitrary fetch now that there is no catalog.
//
// This blocks for as long as the chart takes. The service runs it off the request
// goroutine for that reason.
func (r *Repository) Install(ctx context.Context, spec installSpec) (Release, error) {
	configuration, err := r.clients.configurationFor(spec.Namespace, forWriting)
	if err != nil {
		return Release{}, err
	}

	// A chart may install a CustomResourceDefinition and then an instance of it,
	// which needs the mapper to resolve a kind that did not exist when the cache
	// was filled. Only here: a read costs nothing for a stale cache, and refilling
	// it is dozens of requests.
	r.clients.invalidateDiscovery()

	install := action.NewInstall(configuration)
	install.Namespace = spec.Namespace
	install.ReleaseName = spec.Name
	install.Version = spec.Version
	install.Timeout = r.timeout
	// Wait, so "deployed" means the pods came up rather than that the manifests
	// were accepted. Without it a release reports success while its pods are in
	// ImagePullBackOff, which is the answer a pipeline would act on.
	//
	// Unconditional. There used to be an exception for the release this process
	// was running from, because applying it destroyed the pod doing the waiting.
	// The work runs in a Job now, and a Job is not replaced by the chart it
	// applies, so there is nothing left to except.
	install.WaitStrategy = kube.StatusWatcherStrategy
	install.RollbackOnFailure = spec.RollbackOnFailure
	// Never. The namespace has to exist and be one this lab manages, and letting
	// Helm conjure one would route around the whole protection policy.
	install.CreateNamespace = false

	chart, err := r.locate(&install.ChartPathOptions, spec.Source, spec.Version)
	if err != nil {
		return Release{}, err
	}

	result, err := install.RunWithContext(ctx, chart, spec.Values)
	if err != nil {
		return Release{}, translate(err, "install release "+spec.Name)
	}
	return releaseFrom(result)
}

// Upgrade moves an existing release to another version.
//
// Values that are nil mean "keep what the release already has", which is what
// ReuseValues does and what makes this callable from a pipeline that owns a
// version and nothing else.
func (r *Repository) Upgrade(ctx context.Context, spec upgradeSpec) (Release, error) {
	configuration, err := r.clients.configurationFor(spec.Namespace, forWriting)
	if err != nil {
		return Release{}, err
	}
	r.clients.invalidateDiscovery()

	upgrade := action.NewUpgrade(configuration)
	upgrade.Namespace = spec.Namespace
	upgrade.Version = spec.Version
	upgrade.Timeout = r.timeout
	upgrade.WaitStrategy = kube.StatusWatcherStrategy
	upgrade.RollbackOnFailure = spec.RollbackOnFailure
	upgrade.MaxHistory = maxHistory
	upgrade.ReuseValues = spec.Values == nil

	chart, err := r.locate(&upgrade.ChartPathOptions, spec.Source, spec.Version)
	if err != nil {
		return Release{}, err
	}

	result, err := upgrade.RunWithContext(ctx, spec.Name, chart, spec.Values)
	if err != nil {
		return Release{}, translate(err, "upgrade release "+spec.Name)
	}
	return releaseFrom(result)
}

// Rollback returns a release to an earlier revision.
//
// It creates a new revision rather than restoring the old one, so a rollback is
// itself something that can be rolled back.
func (r *Repository) Rollback(_ context.Context, namespace, name string, revision int) error {
	configuration, err := r.clients.configurationFor(namespace, forWriting)
	if err != nil {
		return err
	}

	rollback := action.NewRollback(configuration)
	rollback.Version = revision
	rollback.Timeout = r.timeout
	rollback.WaitStrategy = kube.StatusWatcherStrategy
	rollback.MaxHistory = maxHistory

	return translate(rollback.Run(name), "roll back release "+name)
}

// Uninstall removes a release and everything it created.
func (r *Repository) Uninstall(_ context.Context, namespace, name string) error {
	configuration, err := r.clients.configurationFor(namespace, forWriting)
	if err != nil {
		return err
	}

	uninstall := action.NewUninstall(configuration)
	uninstall.Timeout = r.timeout
	uninstall.WaitStrategy = kube.StatusWatcherStrategy

	_, err = uninstall.Run(name)
	return translate(err, "uninstall release "+name)
}

// locate resolves a chart source and exact version to a loaded chart.
//
// RepoURL is set rather than a repository being added to any on-disk
// configuration, so Helm fetches the index directly and this pod keeps no
// repository state. The name handed to LocateChart is a bare chart name for an
// HTTP repository and a full oci:// reference for a registry.
func (r *Repository) locate(options *action.ChartPathOptions, source ChartSource,
	version string,
) (*chartv2.Chart, error) {
	options.Version = version

	name := source.Chart
	if source.OCI {
		name = strings.TrimSuffix(source.URL, "/") + "/" + source.Chart
	} else {
		options.RepoURL = source.URL
	}

	path, err := options.LocateChart(name, cli.New())
	if err != nil {
		// Not a bad-request error, even though "could not find the chart" is what
		// this reads like. The service checked the version against the
		// repository's own listing before calling this, so by here the version is
		// known to be offered — which makes a failure a fetch problem: the
		// registry refused the credentials, the index moved, the network went
		// away. Reporting it as a bad request blames the caller for somebody
		// else's outage and sends them looking at their version number.
		//
		// The cause is wrapped rather than dropped. It is the only description of
		// what actually went wrong, and without it every one of those failures
		// reads identically in the log.
		return nil, fmt.Errorf("%w: fetching %s at version %s: %w",
			ErrRepositoryUnreachable, source.Chart, version, err)
	}

	chart, err := loader.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load the chart %s: %w", source.Chart, err)
	}
	return chart, nil
}
