package helm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// DeclareRequest asks this lab to remember a chart for a namespace.
//
// Declaring is not deploying. It writes a record and its first version and puts
// nothing on the cluster, so a half-written values file is a saved draft rather
// than a failed install.
type DeclareRequest struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	ChartRef  string `json:"chartRef"`
	Version   string `json:"version"`
	// ValuesYAML may be empty, which means the chart's own defaults.
	ValuesYAML string `json:"valuesYaml"`
}

// VersionRequest adds a version to a deployment without rolling it out.
type VersionRequest struct {
	Version    string `json:"version"`
	ValuesYAML string `json:"valuesYaml"`
}

// RolloutRequest applies a declared version to the cluster.
type RolloutRequest struct {
	// Version is the declared version to apply, and zero means the newest. Naming
	// one is how "restore what we had on Tuesday" works without editing anything.
	Version int `json:"version,omitempty"`
	// RollbackOnFailure is off by default, and this is the one default worth
	// arguing about. With it on, a failed rollout is undone and the release ends
	// up deployed on a *new* revision — so a pipeline polling for a terminal
	// status reads success for a deploy that did not deploy.
	RollbackOnFailure bool `json:"rollbackOnFailure,omitempty"`
}

// PipelineRequest is the body a pipeline sends: a chart version, and overrides.
//
// Overrides are merged over the newest version's values rather than replacing
// them, and the result is stored as a new version. That is what lets a pipeline
// own an image tag while the operator owns everything else, without either
// erasing the other.
type PipelineRequest struct {
	Version           string         `json:"version"`
	Values            map[string]any `json:"values,omitempty"`
	RollbackOnFailure bool           `json:"rollbackOnFailure,omitempty"`
}

// ListDeployments returns the declared deployments with their live status.
func (s *Service) ListDeployments(ctx context.Context, namespace string) ([]DeploymentSummary, error) {
	if s.store == nil {
		return nil, ErrNotConfigured
	}
	if namespace != "" {
		if err := s.checkNamespace(namespace); err != nil {
			return nil, err
		}
	}

	deployments, err := s.store.ListDeployments(ctx, namespace)
	if err != nil {
		return nil, err
	}

	summaries := make([]DeploymentSummary, 0, len(deployments))
	for _, deployment := range deployments {
		summary, err := s.summarise(ctx, deployment)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

// ReadDeployment returns one deployment, its versions, and its live release.
func (s *Service) ReadDeployment(ctx context.Context, id string) (DeploymentDetail, error) {
	deployment, versions, err := s.load(ctx, id)
	if err != nil {
		return DeploymentDetail{}, err
	}

	detail := DeploymentDetail{Versions: versions}
	detail.Deployment = deployment
	detail.Current = versions[0]

	release, readErr := s.repo.ReadRelease(ctx, deployment.Namespace, deployment.ReleaseName)
	switch {
	case readErr == nil:
		copied := release
		detail.Release = &copied
		detail.Status = release.Status
		detail.State = describeState(detail.Current, &release.Release, false)
	case errors.Is(readErr, ErrNotFound):
		detail.State = describeState(detail.Current, nil, false)
	default:
		// Said out loud rather than swallowed. A refused Secret read presented as
		// "not installed" invites an operator to install a second copy of
		// something that is already running.
		detail.ReleaseError = readErr.Error()
		detail.State = describeState(detail.Current, nil, true)
	}
	return detail, nil
}

// Declare records a chart for a namespace, and puts nothing on the cluster.
func (s *Service) Declare(ctx context.Context, req DeclareRequest, actor string) (Deployment, error) {
	if s.store == nil {
		return Deployment{}, ErrNotConfigured
	}
	if err := s.checkRelease(req.Namespace, req.Name); err != nil {
		return Deployment{}, err
	}

	source, err := ParseChartRef(req.ChartRef)
	if err != nil {
		return Deployment{}, err
	}
	if err := validateVersion(req.Version); err != nil {
		return Deployment{}, err
	}
	if _, err := parseValues(req.ValuesYAML); err != nil {
		return Deployment{}, err
	}

	return s.store.CreateDeployment(ctx,
		Deployment{
			Namespace:   req.Namespace,
			ReleaseName: req.Name,
			// Stored as the parsed reference rebuilds it, so what is recorded is
			// what will be fetched rather than whatever spacing was typed.
			ChartRef:  source.Ref(),
			CreatedBy: actor,
		},
		DeploymentVersion{
			ChartVersion: req.Version,
			ValuesYAML:   req.ValuesYAML,
			Source:       SourcePanel,
			CreatedBy:    actor,
		})
}

// Forget removes the record and leaves the release alone.
//
// Through load like every other deployment path, so the namespace rule is
// applied here too. Reading the record directly would have made this the one
// route that could act on a deployment in a namespace this lab no longer
// manages — the narrowest possible gap, and exactly the kind that survives.
func (s *Service) Forget(ctx context.Context, id string) error {
	deployment, _, err := s.load(ctx, id)
	if err != nil {
		return err
	}
	return s.store.DeleteDeployment(ctx, deployment.ID)
}

// ListVersions returns every declared version, newest first.
func (s *Service) ListVersions(ctx context.Context, id string) ([]DeploymentVersion, error) {
	_, versions, err := s.load(ctx, id)
	return versions, err
}

// AddVersion declares another version without rolling it out.
func (s *Service) AddVersion(ctx context.Context, id string, req VersionRequest,
	actor string,
) (DeploymentVersion, error) {
	deployment, _, err := s.load(ctx, id)
	if err != nil {
		return DeploymentVersion{}, err
	}
	if err := validateVersion(req.Version); err != nil {
		return DeploymentVersion{}, err
	}
	if _, err := parseValues(req.ValuesYAML); err != nil {
		return DeploymentVersion{}, err
	}

	return s.store.AppendVersion(ctx, deployment.ID, DeploymentVersion{
		ChartVersion: req.Version,
		ValuesYAML:   req.ValuesYAML,
		Source:       SourcePanel,
		CreatedBy:    actor,
	})
}

// load reads a deployment and its versions, and refuses a namespace this slice
// may not touch.
//
// Every deployment path starts here, so the namespace rule is applied once
// rather than remembered in seven places. A deployment always has at least one
// version — they are written together — so the caller may index the first.
func (s *Service) load(ctx context.Context, id string) (Deployment, []DeploymentVersion, error) {
	if s.store == nil {
		return Deployment{}, nil, ErrNotConfigured
	}

	deployment, err := s.store.ReadDeployment(ctx, id)
	if err != nil {
		return Deployment{}, nil, err
	}
	if err := s.checkNamespace(deployment.Namespace); err != nil {
		return Deployment{}, nil, err
	}

	versions, err := s.store.ListVersions(ctx, deployment.ID)
	if err != nil {
		return Deployment{}, nil, err
	}
	if len(versions) == 0 {
		// Not reachable through this API — a deployment and its first version are
		// written in one transaction — so if it happens the row was edited by
		// hand, and guessing at what was meant would be worse than saying so.
		return Deployment{}, nil, fmt.Errorf(
			"%w: deployment %s has no versions", ErrNotFound, id)
	}
	return deployment, versions, nil
}

// summarise puts a deployment beside its live release for a listing.
//
// A release that cannot be read does not fail the listing: one unreadable
// namespace would otherwise blank the whole page, and the row can say "unknown"
// perfectly well on its own.
func (s *Service) summarise(ctx context.Context, deployment Deployment) (DeploymentSummary, error) {
	versions, err := s.store.ListVersions(ctx, deployment.ID)
	if err != nil {
		return DeploymentSummary{}, err
	}
	if len(versions) == 0 {
		return DeploymentSummary{}, fmt.Errorf(
			"%w: deployment %s has no versions", ErrNotFound, deployment.ID)
	}

	summary := DeploymentSummary{Deployment: deployment, Current: versions[0]}

	release, readErr := s.repo.ReadRelease(ctx, deployment.Namespace, deployment.ReleaseName)
	switch {
	case readErr == nil:
		summary.Status = release.Status
		summary.State = describeState(summary.Current, &release.Release, false)
	case errors.Is(readErr, ErrNotFound):
		summary.State = describeState(summary.Current, nil, false)
	default:
		s.logger.Warn("could not read a release for a declared deployment",
			slog.String("namespace", deployment.Namespace),
			slog.String("release", deployment.ReleaseName),
			slog.Any("error", readErr))
		summary.State = describeState(summary.Current, nil, true)
	}
	return summary, nil
}
