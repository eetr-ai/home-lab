package helm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"helm.sh/helm/v4/pkg/action"
	chartapi "helm.sh/helm/v4/pkg/chart"
	"helm.sh/helm/v4/pkg/release"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
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
}

// NewRepository builds the repository. It contacts nothing.
func NewRepository(logger *slog.Logger) (*Repository, error) {
	clients, err := newClients(logger)
	if err != nil {
		return nil, err
	}
	return &Repository{clients: clients, logger: logger}, nil
}

// ListReleases returns the releases in each of the given namespaces.
//
// One listing per namespace rather than one cluster-wide listing, because a
// cluster-wide one needs a cluster-wide grant on Secrets — and Helm keeps a
// release in a Secret. Asking each namespace separately is what lets the grant be
// a Role in that namespace and nothing more.
//
// A namespace that cannot be read is logged and skipped rather than failing the
// whole request. The usual cause is a namespace named in configuration that does
// not exist yet, or one whose Role has not been applied, and answering "nothing
// at all" for every other namespace because of it would be the wrong trade.
func (r *Repository) ListReleases(ctx context.Context, namespaces []string) ([]Release, error) {
	releases := []Release{}

	for _, namespace := range namespaces {
		found, err := r.listNamespace(namespace)
		if err != nil {
			r.logger.Warn("could not list helm releases",
				slog.String("namespace", namespace), slog.Any("error", err))
			continue
		}
		releases = append(releases, found...)
	}

	sort.Slice(releases, func(a, b int) bool {
		if releases[a].Namespace != releases[b].Namespace {
			return releases[a].Namespace < releases[b].Namespace
		}
		return releases[a].Name < releases[b].Name
	})
	return releases, nil
}

func (r *Repository) listNamespace(namespace string) ([]Release, error) {
	configuration, err := r.clients.configurationFor(namespace)
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
	configuration, err := r.clients.configurationFor(namespace)
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
	configuration, err := r.clients.configurationFor(namespace)
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
// MetadataAsMap rather than the concrete metadata struct, for the same reason as
// above: it is what the accessor interface offers, so it works for every release
// shape. A field that is missing or is not a string comes back empty, which reads
// as "unknown" in the panel rather than failing the whole listing.
func chartMetadata(charter chartapi.Charter) (name, version, appVersion string) {
	accessor, err := chartapi.NewAccessor(charter)
	if err != nil {
		return "", "", ""
	}

	metadata := accessor.MetadataAsMap()
	return stringField(metadata, "name"), stringField(metadata, "version"),
		stringField(metadata, "appVersion")
}

func stringField(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return value
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
