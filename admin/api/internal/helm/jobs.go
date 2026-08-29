package helm

import (
	"context"
	"fmt"
	"log/slog"
)

// Install puts a new release on the cluster.
//
// Every rule is checked before this returns, so a bad request is a 400 rather
// than a 202 followed by silence. What is not done before it returns is the
// install itself.
func (s *Service) Install(ctx context.Context, req InstallRequest) (Accepted, error) {
	if err := s.checkNamespace(req.Namespace); err != nil {
		return Accepted{}, err
	}
	if err := validateReleaseName(req.Name); err != nil {
		return Accepted{}, err
	}
	if err := validateValues(req.Values); err != nil {
		return Accepted{}, err
	}

	source, err := s.resolve(ctx, req.Chart, req.Version)
	if err != nil {
		return Accepted{}, err
	}

	// Checked so an operator who meant to upgrade is told so, rather than
	// discovering it from whatever Helm says about a name already in use.
	if _, err := s.repo.ReadRelease(ctx, req.Namespace, req.Name); err == nil {
		return Accepted{}, fmt.Errorf("%w: %s already exists in %s",
			ErrAlreadyExists, req.Name, req.Namespace)
	}

	return s.accept(ctx, req.Namespace, req.Name, "install", func(jobCtx context.Context) error {
		_, err := s.repo.Install(jobCtx, req, source)
		return err
	})
}

// Upgrade moves a release to another version.
//
// This is the route a pipeline calls. Its body carries a version and, normally,
// nothing else: absent values mean the release keeps its own, so a pipeline that
// owns an image tag does not have to know or reproduce the rest of the
// configuration — and cannot accidentally erase it.
func (s *Service) Upgrade(ctx context.Context, req UpgradeRequest) (Accepted, error) {
	if err := s.checkNamespace(req.Namespace); err != nil {
		return Accepted{}, err
	}
	if err := validateReleaseName(req.Name); err != nil {
		return Accepted{}, err
	}
	if err := validateValues(req.Values); err != nil {
		return Accepted{}, err
	}

	// Which chart this release came from is read from Helm's storage rather than
	// taken from the request: a pipeline names a version, and letting it also name
	// a chart would let an upgrade quietly replace a release with something else.
	current, err := s.repo.ReadRelease(ctx, req.Namespace, req.Name)
	if err != nil {
		return Accepted{}, err
	}

	source, err := s.resolveInstalled(ctx, current.Chart, req.Version)
	if err != nil {
		return Accepted{}, err
	}

	return s.accept(ctx, req.Namespace, req.Name, "upgrade", func(jobCtx context.Context) error {
		_, err := s.repo.Upgrade(jobCtx, req, source)
		return err
	})
}

// Rollback returns a release to an earlier revision.
func (s *Service) Rollback(ctx context.Context, namespace, name string, revision int) (Accepted, error) {
	if err := s.checkNamespace(namespace); err != nil {
		return Accepted{}, err
	}
	if err := validateReleaseName(name); err != nil {
		return Accepted{}, err
	}
	if revision < 1 {
		return Accepted{}, fmt.Errorf("%w: a revision to roll back to is required", ErrInvalidName)
	}

	history, err := s.repo.ReadHistory(ctx, namespace, name)
	if err != nil {
		return Accepted{}, err
	}
	if !hasRevision(history, revision) {
		return Accepted{}, fmt.Errorf("%w: %s has no revision %d", ErrNotFound, name, revision)
	}

	return s.accept(ctx, namespace, name, "rollback", func(jobCtx context.Context) error {
		return s.repo.Rollback(jobCtx, namespace, name, revision)
	})
}

// Uninstall removes a release and everything it created.
func (s *Service) Uninstall(ctx context.Context, namespace, name string) (Accepted, error) {
	if err := s.checkNamespace(namespace); err != nil {
		return Accepted{}, err
	}
	if err := validateReleaseName(name); err != nil {
		return Accepted{}, err
	}
	if _, err := s.repo.ReadRelease(ctx, namespace, name); err != nil {
		return Accepted{}, err
	}

	return s.accept(ctx, namespace, name, "uninstall", func(jobCtx context.Context) error {
		return s.repo.Uninstall(jobCtx, namespace, name)
	})
}

// accept takes the release's lock and runs the operation off the request.
//
// Helm waits for pods, and that outlasts every timeout between the browser and
// here: the panel gives up at twenty seconds and the HTTP server stops writing at
// thirty. So the request is answered as soon as the rules have passed and the work
// is detached with context.WithoutCancel — the caller hanging up must not cancel
// an install that is already applying manifests, which is how a release ends up
// half-applied and wedged.
//
// There is no job id, because there is nothing to look one up in. The outcome is
// read back out of Helm's storage through the release endpoint, which is the same
// place both replicas read it from, and which is exactly what having no database
// buys.
func (s *Service) accept(ctx context.Context, namespace, name, operation string,
	run func(context.Context) error,
) (Accepted, error) {
	if !s.locks.acquire(namespace, name) {
		return Accepted{}, fmt.Errorf("%w: %s in %s", ErrInProgress, name, namespace)
	}

	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.timeout)
	go func() {
		defer cancel()
		defer s.locks.release(namespace, name)

		if err := run(jobCtx); err != nil {
			// The only place this is reported. There is no caller left to tell,
			// and the release itself carries the outcome — Helm records the
			// failure and its reason on the revision.
			s.logger.Error("a helm operation failed",
				slog.String("operation", operation),
				slog.String("namespace", namespace),
				slog.String("release", name),
				slog.Any("error", err))
		}
	}()

	return Accepted{
		Namespace: namespace,
		Release:   name,
		Operation: operation,
		Message: "accepted; read the release to see whether it succeeded — it is " +
			operation + "ing until its status is no longer pending",
	}, nil
}

// resolve checks a chart and version against the catalog and the repository.
//
// Both halves are needed. The catalog decides whether this lab installs the chart
// at all and whether it permits the version; the repository decides whether the
// version exists. A pin naming a version that was yanked passes the first and
// fails the second, which is the right answer.
func (s *Service) resolve(ctx context.Context, name, version string) (ChartSource, error) {
	if !s.catalog.Configured() {
		return ChartSource{}, ErrNotConfigured
	}
	if err := validateVersion(version); err != nil {
		return ChartSource{}, err
	}

	chart, source, err := s.catalog.Find(name)
	if err != nil {
		return ChartSource{}, err
	}

	offered, err := s.repo.ListChartVersions(ctx, source)
	if err != nil {
		// Refused rather than attempted. An unreachable repository is a fine
		// reason to show a stale catalog and a bad reason to install something
		// nothing could confirm.
		return ChartSource{}, err
	}
	if !chart.permits(version, offered) {
		return ChartSource{}, fmt.Errorf("%w: %s at version %s", ErrUnknownVersion, name, version)
	}
	return source, nil
}

// resolveInstalled finds the catalog entry a running release came from.
//
// A release's stored chart name is the chart's own name, not the catalog key, so
// this looks the entry up by what it points at. A release installed from
// somewhere else has no entry and cannot be upgraded from here — the panel can
// see everything Helm put in a managed namespace and can only change what was
// vetted.
func (s *Service) resolveInstalled(ctx context.Context, chartName, version string) (ChartSource, error) {
	if !s.catalog.Configured() {
		return ChartSource{}, ErrNotConfigured
	}

	for _, entry := range s.catalog.Charts {
		if entry.Chart == chartName {
			return s.resolve(ctx, entry.Name, version)
		}
	}
	return ChartSource{}, fmt.Errorf(
		"%w: this release was installed from %s, which this lab's catalog does not list",
		ErrUnknownChart, chartName)
}

func hasRevision(history []Revision, revision int) bool {
	for _, entry := range history {
		if entry.Revision == revision {
			return true
		}
	}
	return false
}
