package helm

import (
	"context"
	"errors"
	"log/slog"
)

// Rollout applies a declared version to the cluster.
//
// Accepted, not performed. Every rule is checked before this returns — the
// namespace, the chart reference, the version, the values — so a bad request is a
// 400 rather than a 202 followed by silence.
func (s *Service) Rollout(ctx context.Context, id string, req RolloutRequest) (Accepted, error) {
	deployment, versions, err := s.load(ctx, id)
	if err != nil {
		return Accepted{}, err
	}

	version := versions[0]
	if req.Version != 0 {
		version, err = s.store.ReadVersion(ctx, deployment.ID, req.Version)
		if err != nil {
			return Accepted{}, err
		}
	}
	return s.apply(ctx, deployment, version, req.RollbackOnFailure)
}

// PipelineRollout is the endpoint a pipeline calls: a chart version, and
// overrides to merge over what is already declared.
//
// The overrides do not replace the stored values, and they do not vanish after
// the deploy either — they become a new declared version, so the record still
// describes what is running and the next rollout from the panel does not undo
// the pipeline's work.
func (s *Service) PipelineRollout(ctx context.Context, id string, req PipelineRequest,
	actor string,
) (Accepted, error) {
	deployment, versions, err := s.load(ctx, id)
	if err != nil {
		return Accepted{}, err
	}
	if err := validateVersion(req.Version); err != nil {
		return Accepted{}, err
	}

	document, err := overlay(versions[0], req.Values)
	if err != nil {
		return Accepted{}, err
	}

	version, err := s.store.AppendVersion(ctx, deployment.ID, DeploymentVersion{
		ChartVersion: req.Version,
		ValuesYAML:   document,
		Source:       SourceCI,
		CreatedBy:    actor,
	})
	if err != nil {
		return Accepted{}, err
	}
	return s.apply(ctx, deployment, version, req.RollbackOnFailure)
}

// overlay merges a pipeline's values over the newest declared ones.
//
// With no overrides the previous document is carried through byte for byte, so a
// pipeline that only bumps a chart version does not silently strip the comments
// out of an operator's values file. Only an actual override forces a rewrite,
// and that version is marked as generated in its first line.
func overlay(previous DeploymentVersion, overrides map[string]any) (string, error) {
	if len(overrides) == 0 {
		return previous.ValuesYAML, nil
	}

	base, err := parseValues(previous.ValuesYAML)
	if err != nil {
		return "", err
	}

	document, err := renderValues(mergeValues(base, overrides))
	if err != nil {
		return "", err
	}
	return document, checkValuesSize(document)
}

// apply puts one declared version on the cluster.
//
// Whether that is an install or an upgrade is decided by what Helm has, not by
// which endpoint was called: a deployment whose release was uninstalled installs
// cleanly, and a release that already exists is upgraded. Only "not found" counts
// as absent — treating every failed read as absence would install over a release
// this could not see, and a refused Secret read would present as a clean slate.
func (s *Service) apply(ctx context.Context, deployment Deployment, version DeploymentVersion,
	rollbackOnFailure bool,
) (Accepted, error) {
	source, err := ParseChartRef(deployment.ChartRef)
	if err != nil {
		return Accepted{}, err
	}
	if err := validateVersion(version.ChartVersion); err != nil {
		return Accepted{}, err
	}
	values, err := parseValues(version.ValuesYAML)
	if err != nil {
		return Accepted{}, err
	}

	installing := false
	switch _, err := s.repo.ReadRelease(ctx, deployment.Namespace, deployment.ReleaseName); {
	case err == nil:
	case errors.Is(err, ErrNotFound):
		installing = true
	default:
		return Accepted{}, err
	}

	operation := "upgrade"
	if installing {
		operation = "install"
	}

	// An operation on the release this process is running from cannot wait for
	// the workloads it applies, because one of them is this pod. See
	// waitStrategy — the short version is that waiting would leave the release
	// wedged and refuse every later deploy.
	self := s.self.Matches(deployment.Namespace, deployment.ReleaseName)
	if self {
		s.logger.Info("this operation targets the panel's own release; not waiting for readiness",
			slog.String("namespace", deployment.Namespace),
			slog.String("release", deployment.ReleaseName))
	}

	accepted, err := s.accept(ctx, deployment.Namespace, deployment.ReleaseName, operation,
		func(jobCtx context.Context) error {
			return s.run(jobCtx, deployment, version, applySpec{
				source:            source,
				values:            values,
				installing:        installing,
				rollbackOnFailure: rollbackOnFailure,
				skipWait:          self,
			})
		})
	if err != nil {
		return Accepted{}, err
	}

	// Said in the response, not only in the log. Whoever called this is about to
	// poll for a status that will mean less than usual, and the difference is
	// theirs to know about.
	if self {
		accepted.Message = "accepted, not performed; this is the panel's own release, so it " +
			"is recorded as deployed once the manifests are applied rather than once the new " +
			"pods are ready — check the workload itself, not just the release status"
	}
	return accepted, nil
}

// applySpec is what apply worked out and run carries out.
type applySpec struct {
	source            ChartSource
	values            map[string]any
	installing        bool
	rollbackOnFailure bool
	skipWait          bool
}

// run performs the Helm operation and records that the version reached the
// cluster.
//
// The stamp is written only on success, and a failure to write it is logged
// rather than returned: the release is already up, and reporting the rollout as
// failed because the bookkeeping failed would be a worse lie than a record that
// briefly reads "not rolled out". The panel would then show drift, which is
// recoverable by rolling out again.
func (s *Service) run(ctx context.Context, deployment Deployment, version DeploymentVersion,
	spec applySpec,
) error {
	var (
		release Release
		err     error
	)

	if spec.installing {
		release, err = s.repo.Install(ctx, installSpec{
			Namespace:         deployment.Namespace,
			Name:              deployment.ReleaseName,
			Source:            spec.source,
			Version:           version.ChartVersion,
			Values:            spec.values,
			RollbackOnFailure: spec.rollbackOnFailure,
			SkipWait:          spec.skipWait,
		})
	} else {
		release, err = s.repo.Upgrade(ctx, upgradeSpec{
			Namespace:         deployment.Namespace,
			Name:              deployment.ReleaseName,
			Source:            spec.source,
			Version:           version.ChartVersion,
			Values:            spec.values,
			RollbackOnFailure: spec.rollbackOnFailure,
			SkipWait:          spec.skipWait,
		})
	}
	if err != nil {
		return err
	}

	if err := s.store.MarkRolledOut(ctx, deployment.ID, version.Version, release.Revision); err != nil {
		s.logger.Error("the rollout succeeded but could not be recorded",
			slog.String("deployment", deployment.ID),
			slog.Int("version", version.Version),
			slog.Any("error", err))
	}
	return nil
}
