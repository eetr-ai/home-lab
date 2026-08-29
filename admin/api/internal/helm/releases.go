package helm

import (
	"context"
	"fmt"
)

// Rollback returns a release to an earlier revision.
func (s *Service) Rollback(ctx context.Context, namespace, name string, revision int,
	actor string,
) (Accepted, error) {
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

	return s.dispatch(ctx, JobSpec{
		Operation: OpRollback,
		Namespace: namespace,
		Release:   name,
		Revision:  revision,
	}, ReleaseRef{Namespace: namespace, Release: name}, actor)
}

// Uninstall removes a release and everything it created.
//
// The deployment record, if there is one, is deliberately left behind: an
// operator who uninstalls a release usually means "take it off the cluster", not
// "forget the values I wrote". Forgetting is its own, separate request.
func (s *Service) Uninstall(ctx context.Context, namespace, name string,
	actor string,
) (Accepted, error) {
	if err := s.checkRelease(namespace, name); err != nil {
		return Accepted{}, err
	}
	if _, err := s.repo.ReadRelease(ctx, namespace, name); err != nil {
		return Accepted{}, err
	}

	return s.dispatch(ctx, JobSpec{
		Operation: OpUninstall,
		Namespace: namespace,
		Release:   name,
	}, ReleaseRef{Namespace: namespace, Release: name}, actor)
}

// dispatch hands one operation to a Job and answers as soon as it exists.
//
// Accepted, not performed. Helm waits for pods, and that outlasts every timeout
// between the browser and here: the panel gives up at twenty seconds and the HTTP
// server stops writing at thirty. What changed is where the work goes. It used to
// be a goroutine in this process, which meant the deploy died with the pod running
// it — and meant an upgrade of the panel's own chart could not wait for readiness,
// because the pod doing the waiting was one of the ones being replaced.
//
// A Job is not replaced by the thing it is applying. So the wait is real for every
// release, and the answer now carries the name of an object that has a status and
// somewhere to put its logs, rather than asking the caller to infer the outcome
// from Helm's storage.
//
// Every rule is checked before this is reached — the namespace, the chart
// reference, the version, the values — so a bad request is a 400 rather than a 202
// followed by a pod that fails two seconds later.
func (s *Service) dispatch(ctx context.Context, spec JobSpec, ref ReleaseRef,
	actor string,
) (Accepted, error) {
	if s.jobs == nil {
		return Accepted{}, ErrNotConfigured
	}

	running, err := s.jobs.ListJobs(ctx, JobFilter{
		Namespace: ref.Namespace,
		Release:   ref.Release,
	})
	if err != nil {
		return Accepted{}, err
	}
	if active := activeJob(running); active != nil {
		return Accepted{}, fmt.Errorf("%w: %s is already running a %s of %s in %s",
			ErrInProgress, active.Name, active.Operation, ref.Release, ref.Namespace)
	}

	job, err := s.jobs.CreateJob(ctx, spec, ref, actor)
	if err != nil {
		return Accepted{}, err
	}

	return Accepted{
		Namespace: ref.Namespace,
		Release:   ref.Release,
		Operation: spec.Operation,
		Job:       job.Name,
		Message: "accepted, not performed; job " + job.Name +
			" is doing the work — read /api/helm/jobs/" + job.Name +
			" for its status, or follow its events",
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
