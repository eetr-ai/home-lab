package helm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// RunJob performs one Helm operation and returns when it is done.
//
// This is the whole of what the Kubernetes Job runs. It is in this package rather
// than in a sibling because it needs installSpec, upgradeSpec, parseValues and the
// repository's write methods — all unexported, and all belonging to this slice. A
// sibling package would have to reach across the seam for them, which is the one
// thing the layering rule forbids, and exporting five internal types so that one
// caller could reach them would be the same violation wearing a disguise.
//
// It blocks for as long as Helm takes, waiting for the workloads it applied to
// become ready. That is the point of running here at all: nothing is replacing
// this process while it waits, so the wait can be real — including when the chart
// being applied is the panel's own.
//
// The error it returns becomes the pod's exit code, which becomes the Job's
// status, which is what the panel and a pipeline read. So a Helm failure must
// reach this as an error and must not be swallowed on the way.
func RunJob(ctx context.Context, args []string, logger *slog.Logger) error {
	spec, err := parseJobArgs(args)
	if err != nil {
		return err
	}

	timeout, err := Timeout()
	if err != nil {
		return err
	}

	// The whole operation is bounded, not just the Helm call. A runner wedged
	// somewhere Helm's own timeout does not cover would otherwise hold the release
	// pending until the Job's activeDeadlineSeconds fired, which is a less legible
	// failure than this one.
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	repo, err := NewRepository(logger, timeout)
	if err != nil {
		return err
	}

	logger.Info("performing a helm operation",
		slog.String("operation", spec.Operation),
		slog.String("namespace", spec.Namespace),
		slog.String("release", spec.Release),
		slog.String("deployment", spec.DeploymentID),
		slog.Duration("timeout", timeout))

	switch spec.Operation {
	case OpRollout:
		return runRollout(ctx, repo, spec, logger)
	case OpRollback:
		return repo.Rollback(ctx, spec.Namespace, spec.Release, spec.Revision)
	case OpUninstall:
		return repo.Uninstall(ctx, spec.Namespace, spec.Release)
	default:
		return fmt.Errorf("%w: %q is not a Helm operation", ErrInvalidName, spec.Operation)
	}
}

// runRollout applies one declared version to the cluster and records that it
// arrived.
//
// Whether that is an install or an upgrade is decided here, from what Helm has,
// rather than by which endpoint was called: a deployment whose release was
// uninstalled installs cleanly, and a release that already exists is upgraded.
// Only "not found" counts as absent — treating every failed read as absence would
// install over a release this could not see, and a refused Secret read would
// present as a clean slate.
func runRollout(ctx context.Context, repo *Repository, spec JobSpec, logger *slog.Logger) error {
	store, err := openStore(ctx, logger)
	if err != nil {
		return err
	}
	defer store.Close()

	deployment, err := store.ReadDeployment(ctx, spec.DeploymentID)
	if err != nil {
		return err
	}
	version, err := store.ReadVersion(ctx, spec.DeploymentID, spec.Version)
	if err != nil {
		return err
	}

	source, err := ParseChartRef(deployment.ChartRef)
	if err != nil {
		return err
	}
	if err := validateVersion(version.ChartVersion); err != nil {
		return err
	}
	values, err := parseValues(version.ValuesYAML)
	if err != nil {
		return err
	}

	installing := false
	switch _, err := repo.ReadRelease(ctx, deployment.Namespace, deployment.ReleaseName); {
	case err == nil:
	case errors.Is(err, ErrNotFound):
		installing = true
	default:
		return err
	}

	var release Release
	if installing {
		logger.Info("installing", slog.String("chart", deployment.ChartRef),
			slog.String("chartVersion", version.ChartVersion))
		release, err = repo.Install(ctx, installSpec{
			Namespace:         deployment.Namespace,
			Name:              deployment.ReleaseName,
			Source:            source,
			Version:           version.ChartVersion,
			Values:            values,
			RollbackOnFailure: spec.RollbackOnFailure,
		})
	} else {
		logger.Info("upgrading", slog.String("chart", deployment.ChartRef),
			slog.String("chartVersion", version.ChartVersion))
		release, err = repo.Upgrade(ctx, upgradeSpec{
			Namespace:         deployment.Namespace,
			Name:              deployment.ReleaseName,
			Source:            source,
			Version:           version.ChartVersion,
			Values:            values,
			RollbackOnFailure: spec.RollbackOnFailure,
		})
	}
	if err != nil {
		return err
	}

	// The stamp is written only on success, and a failure to write it is logged
	// rather than returned. The release is already up: reporting the rollout as
	// failed because the bookkeeping failed would be a worse lie than a record
	// that briefly reads "not rolled out", and it is now a worse one than it used
	// to be, because this process's exit code is what the panel and the pipeline
	// read as the outcome. The panel shows drift instead, which is true and is
	// recoverable by rolling out again.
	if err := store.MarkRolledOut(ctx, deployment.ID, version.Version, release.Revision); err != nil {
		logger.Error("the rollout succeeded but could not be recorded",
			slog.String("deployment", deployment.ID),
			slog.Int("version", version.Version),
			slog.Any("error", err))
	}

	logger.Info("the release is up",
		slog.String("release", release.Name),
		slog.Int("revision", release.Revision),
		slog.String("status", release.Status))
	return nil
}

// openStore opens the record of what this lab declared.
//
// Fatal here, unlike in the API. The API can serve every other page without a
// database and answers 501 for the ones that need it; a rollout has nothing left
// to do without one, and starting the Helm operation anyway would apply a chart
// and then be unable to record that it had.
func openStore(ctx context.Context, logger *slog.Logger) (*Store, error) {
	dsn := os.Getenv("ADMIN_HELM_DSN")
	if dsn == "" {
		return nil, fmt.Errorf(
			"ADMIN_HELM_DSN is unset, and a rollout reads the values to apply from it")
	}
	return NewStore(ctx, dsn, logger)
}

// DefaultTimeout is how long one Helm operation may take when ADMIN_HELM_TIMEOUT
// says nothing.
//
// Generous, because it covers pulling a chart, applying it, and waiting for the
// pods to become ready. What it protects against is an operation that never
// finishes holding a release in a pending state forever.
const DefaultTimeout = 10 * time.Minute

// Timeout bounds one install, upgrade, rollback, or uninstall.
//
// Read from the environment in both processes — the API sizes the Job's deadline
// with it, and the Job bounds itself with it — so the chart sets it once and the
// two cannot disagree.
func Timeout() (time.Duration, error) {
	value := os.Getenv("ADMIN_HELM_TIMEOUT")
	if value == "" {
		return DefaultTimeout, nil
	}

	timeout, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("ADMIN_HELM_TIMEOUT is not a duration: %w", err)
	}
	if timeout <= 0 {
		return 0, fmt.Errorf("ADMIN_HELM_TIMEOUT must be positive, and is %s", timeout)
	}
	return timeout, nil
}
