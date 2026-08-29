package helm

import (
	"context"
	"errors"
)

// Rollout applies a declared version to the cluster.
//
// Accepted, not performed. Every rule is checked before this returns — the
// namespace, the chart reference, the version, the values — so a bad request is a
// 400 rather than a 202 followed by silence.
func (s *Service) Rollout(ctx context.Context, id string, req RolloutRequest,
	actor string,
) (Accepted, error) {
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
	return s.apply(ctx, deployment, version, req.RollbackOnFailure, actor)
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
	return s.apply(ctx, deployment, version, req.RollbackOnFailure, actor)
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
// Everything that can be a 4xx is checked here, synchronously, before a Job is
// created: the chart reference, the version, the values. What is deliberately NOT
// checked here is whether the release already exists — the Job decides install
// versus upgrade from what Helm has when the work starts, because between this
// answering 202 and the pod starting, a release can appear or vanish, and a guess
// made now would be a second answer to a question the cluster is about to be asked
// again.
func (s *Service) apply(ctx context.Context, deployment Deployment, version DeploymentVersion,
	rollbackOnFailure bool, actor string,
) (Accepted, error) {
	if _, err := ParseChartRef(deployment.ChartRef); err != nil {
		return Accepted{}, err
	}
	if err := validateVersion(version.ChartVersion); err != nil {
		return Accepted{}, err
	}
	// Parsed and thrown away. The Job reads the values from the record itself, so
	// this is not how they travel — but "your YAML is broken" has to be a 400 in
	// front of whoever wrote it, not a pod that fails two seconds later.
	if _, err := parseValues(version.ValuesYAML); err != nil {
		return Accepted{}, err
	}

	// The release is read, and the answer is deliberately discarded. Only the
	// failure is used: a namespace whose Secrets this cannot read is a 403 now,
	// in front of whoever asked, rather than a Job that starts and dies. Absence
	// is fine and means nothing here — it is the Job's business whether that turns
	// into an install.
	switch _, err := s.repo.ReadRelease(ctx, deployment.Namespace, deployment.ReleaseName); {
	case err == nil, errors.Is(err, ErrNotFound):
	default:
		return Accepted{}, err
	}

	return s.dispatch(ctx, JobSpec{
		Operation:         OpRollout,
		DeploymentID:      deployment.ID,
		Version:           version.Version,
		RollbackOnFailure: rollbackOnFailure,
	}, ReleaseRef{Namespace: deployment.Namespace, Release: deployment.ReleaseName}, actor)
}
