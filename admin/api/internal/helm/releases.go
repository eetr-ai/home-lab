package helm

import (
	"context"
	"fmt"
	"log/slog"
)

// Rollback returns a release to an earlier revision.
func (s *Service) Rollback(ctx context.Context, namespace, name string, revision int) (Accepted, error) {
	if err := s.checkRelease(namespace, name); err != nil {
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
//
// The deployment record, if there is one, is deliberately left behind: an
// operator who uninstalls a release usually means "take it off the cluster", not
// "forget the values I wrote". Forgetting is its own, separate request.
func (s *Service) Uninstall(ctx context.Context, namespace, name string) (Accepted, error) {
	if err := s.checkRelease(namespace, name); err != nil {
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
// There is no job id. The outcome is read back out of Helm's storage through the
// release endpoint, which is the same place both replicas read it from — a job
// table would be a second account of something the cluster already knows, and the
// two would disagree the first time a pod was killed mid-operation.
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
		// No gerund built by appending "ing" to the operation: that produced
		// "upgradeing". The sentence says what to do instead of conjugating.
		Message: "accepted, not performed; read the release to see whether the " +
			operation + " succeeded — it is not finished until the status is no longer pending",
	}, nil
}

func hasRevision(history []Revision, revision int) bool {
	for _, entry := range history {
		if entry.Revision == revision {
			return true
		}
	}
	return false
}
