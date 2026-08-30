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
	//
	// Deliberately longer than the timeout Helm itself gets. The two bound
	// different things and the inner one has to win: at exactly equal deadlines
	// this context can cancel an install *while it is applying manifests*, which
	// is the state that is hardest to recover from — and it would do so instead of
	// letting Helm time out cleanly and record the failure on the release.
	ctx, cancel := context.WithTimeout(ctx, timeout+runnerGrace)
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

	deployment, version, source, values, err := readDeclared(ctx, store, spec)
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

	release, err := applyChart(ctx, repo, applyArgs{
		deployment:        deployment,
		source:            source,
		chartVersion:      version.ChartVersion,
		values:            values,
		installing:        installing,
		rollbackOnFailure: spec.RollbackOnFailure,
		logger:            logger,
	})
	if err != nil {
		return err
	}

	stampRollout(ctx, store, deployment, version, release, logger)

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

// runnerGrace is how long the runner may outlive the timeout Helm is given.
//
// The gap exists so Helm's own timeout is always the one that fires. See RunJob.
const runnerGrace = time.Minute

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

// applyArgs is what runRollout worked out and applyChart carries out.
type applyArgs struct {
	deployment        Deployment
	source            ChartSource
	chartVersion      string
	values            map[string]any
	installing        bool
	rollbackOnFailure bool
	logger            *slog.Logger
}

// applyChart installs or upgrades, and blocks until the workloads are ready.
func applyChart(ctx context.Context, repo *Repository, args applyArgs) (Release, error) {
	if args.installing {
		args.logger.Info("installing", slog.String("chart", args.deployment.ChartRef),
			slog.String("chartVersion", args.chartVersion))
		return repo.Install(ctx, installSpec{
			Namespace:         args.deployment.Namespace,
			Name:              args.deployment.ReleaseName,
			Source:            args.source,
			Version:           args.chartVersion,
			Values:            args.values,
			RollbackOnFailure: args.rollbackOnFailure,
		})
	}

	args.logger.Info("upgrading", slog.String("chart", args.deployment.ChartRef),
		slog.String("chartVersion", args.chartVersion))
	return repo.Upgrade(ctx, upgradeSpec{
		Namespace:         args.deployment.Namespace,
		Name:              args.deployment.ReleaseName,
		Source:            args.source,
		Version:           args.chartVersion,
		Values:            args.values,
		RollbackOnFailure: args.rollbackOnFailure,
	})
}

// readDeclared resolves what the record says should be running.
//
// The values come from here rather than from the Job's own arguments, which is
// why the database credential is injected: an operator's values never travel
// through a Job object, and because the version is numbered, one appended between
// the 202 and this running cannot change what gets applied.
func readDeclared(ctx context.Context, store *Store, spec JobSpec) (
	Deployment, DeploymentVersion, ChartSource, map[string]any, error,
) {
	deployment, err := store.ReadDeployment(ctx, spec.DeploymentID)
	if err != nil {
		return Deployment{}, DeploymentVersion{}, ChartSource{}, nil, err
	}
	version, err := store.ReadVersion(ctx, spec.DeploymentID, spec.Version)
	if err != nil {
		return Deployment{}, DeploymentVersion{}, ChartSource{}, nil, err
	}

	source, err := ParseChartRef(deployment.ChartRef)
	if err != nil {
		return Deployment{}, DeploymentVersion{}, ChartSource{}, nil, err
	}
	if err := validateVersion(version.ChartVersion); err != nil {
		return Deployment{}, DeploymentVersion{}, ChartSource{}, nil, err
	}
	values, err := parseValues(version.ValuesYAML)
	if err != nil {
		return Deployment{}, DeploymentVersion{}, ChartSource{}, nil, err
	}
	return deployment, version, source, values, nil
}

// stampWindow is how long the rollout stamp gets, once the release is up.
//
// Short: it is one UPDATE against a database this process has an open pool to.
const stampWindow = 30 * time.Second

// rolloutStamper records that a declared version reached the cluster. Declared
// here, where it is consumed, so the deadline rule below can be tested.
type rolloutStamper interface {
	MarkRolledOut(ctx context.Context, id string, number, helmRevision int) error
}

// stampRollout records that this version reached the cluster.
//
// On a context of its own, detached from the operation's. That is the whole point
// of this function existing: the operation's context is bounded by the Helm
// timeout, and a deploy that legitimately used most of it would arrive here with
// a context already expiring — so the release would be up and the record would
// say it never rolled out. The panel calls that drift, and it would be drift
// reported for a deploy that worked, on exactly the slow deploys most worth
// trusting the record about.
//
// The failure is still logged rather than returned. The release is already up,
// and reporting the rollout as failed because the bookkeeping failed would be a
// worse lie than a record that briefly reads "not rolled out" — worse now than it
// used to be, because this process's exit code is what the panel and a pipeline
// read as the outcome. Drift is true, visible, and fixed by rolling out again.
func stampRollout(ctx context.Context, store rolloutStamper, deployment Deployment,
	version DeploymentVersion, release Release, logger *slog.Logger,
) {
	stampCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), stampWindow)
	defer cancel()

	if err := store.MarkRolledOut(stampCtx, deployment.ID, version.Version, release.Revision); err != nil {
		logger.Error("the rollout succeeded but could not be recorded",
			slog.String("deployment", deployment.ID),
			slog.Int("version", version.Version),
			slog.Any("error", err))
	}
}
