package helm

import (
	"fmt"
	"strconv"
)

// RunCommand is the subcommand that performs one Helm operation and exits.
//
// It lives here rather than in main because Args builds a command line with it
// and main dispatches on it, and the two must agree — a literal in each place is
// a way for them not to.
const RunCommand = "helm-run"

// The operations a Job can be asked to perform.
//
// Three, not four. Whether a rollout installs or upgrades is decided inside the
// Job by asking Helm what it has, because between the API answering 202 and the
// pod starting, a release can appear or vanish — and a decision made at accept
// time would be a second answer to a question the cluster is about to be asked
// again anyway.
const (
	OpRollout   = "rollout"
	OpRollback  = "rollback"
	OpUninstall = "uninstall"
)

// JobSpec is one Helm operation, in the form it survives a process boundary.
//
// It is deliberately four scalars and a bool. Everything else the Job needs — the
// chart reference, the values, the chart version — it reads from the deployment
// record itself, addressed by (deployment id, version number). That is what the
// database credential is injected for, and it has two consequences worth stating:
// an operator's values never travel through a Job object, which is not a Secret;
// and because the version is *numbered*, a version appended between the 202 and
// the pod starting cannot change what gets applied.
//
// This is also the whole of what a caller influences about the Job. The image,
// the ServiceAccount, the namespace and the command all come from the API's own
// configuration — see buildJob, where that invariant is the security property it
// looks like.
type JobSpec struct {
	// Operation is OpRollout, OpRollback, or OpUninstall.
	Operation string
	// DeploymentID and Version address the declared version to apply. Rollout only.
	DeploymentID string
	Version      int
	// Namespace and Release name the release directly, for the two operations that
	// need no deployment record: a release installed by hand can still be rolled
	// back and uninstalled from here.
	Namespace string
	Release   string
	// Revision is which revision to return to. Rollback only.
	Revision int
	// RollbackOnFailure undoes a failed rollout. Rollout only.
	RollbackOnFailure bool
}

// Args renders the spec as the command line the Job runs.
//
// Arguments rather than environment variables, and the split is deliberate: args
// carry the request, env carries the ambient configuration this binary already
// reads with os.Getenv — the database credential, the Helm timeout, the cache
// paths. It also means `kubectl describe job` shows the operation as a command
// somebody can read, rather than as configuration they have to go looking for.
func (s JobSpec) Args() []string {
	args := []string{RunCommand, s.Operation}

	switch s.Operation {
	case OpRollout:
		args = append(args, "--deployment", s.DeploymentID,
			"--version", strconv.Itoa(s.Version))
		if s.RollbackOnFailure {
			args = append(args, "--rollback-on-failure")
		}
	case OpRollback:
		args = append(args, "--namespace", s.Namespace, "--release", s.Release,
			"--revision", strconv.Itoa(s.Revision))
	case OpUninstall:
		args = append(args, "--namespace", s.Namespace, "--release", s.Release)
	}
	return args
}

// parseJobArgs reads back what Args wrote.
//
// Hand-rolled rather than flag.FlagSet because the valid flags differ per
// operation, and a FlagSet that accepted every flag for every operation would
// silently ignore `--revision` on a rollout instead of refusing it. Silently
// ignoring an argument is how a rollback becomes an uninstall of the wrong thing.
//
// The encoder is in this file too, so the round trip is one table test — and that
// matters more than usual here, because the two halves run in different processes
// and a mismatch would otherwise only ever appear inside a pod.
func parseJobArgs(args []string) (JobSpec, error) {
	if len(args) == 0 {
		return JobSpec{}, fmt.Errorf("%w: an operation is required", ErrInvalidName)
	}

	spec := JobSpec{Operation: args[0]}
	switch spec.Operation {
	case OpRollout, OpRollback, OpUninstall:
	default:
		return JobSpec{}, fmt.Errorf("%w: %q is not a Helm operation", ErrInvalidName, spec.Operation)
	}

	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		flag := rest[i]
		if flag == "--rollback-on-failure" {
			spec.RollbackOnFailure = true
			continue
		}

		i++
		if i == len(rest) {
			return JobSpec{}, fmt.Errorf("%w: %s needs a value", ErrInvalidName, flag)
		}
		if err := spec.set(flag, rest[i]); err != nil {
			return JobSpec{}, err
		}
	}

	return spec, spec.validate()
}

// set assigns one argument, refusing a flag this binary does not know.
func (s *JobSpec) set(flag, value string) error {
	var err error
	switch flag {
	case "--deployment":
		s.DeploymentID = value
	case "--namespace":
		s.Namespace = value
	case "--release":
		s.Release = value
	case "--version":
		s.Version, err = strconv.Atoi(value)
	case "--revision":
		s.Revision, err = strconv.Atoi(value)
	default:
		return fmt.Errorf("%w: %q is not a known argument", ErrInvalidName, flag)
	}
	if err != nil {
		return fmt.Errorf("%w: %s must be a number, and is %q", ErrInvalidName, flag, value)
	}
	return nil
}

// validate refuses a spec that names an operation it cannot perform.
//
// Every field is checked against the operation that uses it, and a field set for
// an operation that ignores it is an error rather than surplus. The alternative —
// accepting it and dropping it — means a bug in the API that sends the wrong
// shape produces a Job that does something adjacent to what was asked, which is
// far harder to see than a pod that refuses to start.
func (s JobSpec) validate() error {
	if s.Operation == OpRollout {
		return s.validateRollout()
	}
	return s.validateRelease()
}

func (s JobSpec) validateRollout() error {
	if s.DeploymentID == "" {
		return fmt.Errorf("%w: a rollout needs a deployment", ErrInvalidName)
	}
	if s.Version < 1 {
		return fmt.Errorf("%w: a rollout needs a declared version, and %d is not one",
			ErrInvalidName, s.Version)
	}
	if s.Namespace != "" || s.Release != "" || s.Revision != 0 {
		return fmt.Errorf("%w: a rollout is addressed by deployment, not by release",
			ErrInvalidName)
	}
	return nil
}

func (s JobSpec) validateRelease() error {
	if err := validateNamespace(s.Namespace); err != nil {
		return err
	}
	if err := validateReleaseName(s.Release); err != nil {
		return err
	}
	if s.DeploymentID != "" || s.Version != 0 || s.RollbackOnFailure {
		return fmt.Errorf("%w: a %s is addressed by release, not by deployment",
			ErrInvalidName, s.Operation)
	}
	if s.Operation == OpRollback && s.Revision < 1 {
		return fmt.Errorf("%w: a rollback needs a revision to return to", ErrInvalidName)
	}
	if s.Operation == OpUninstall && s.Revision != 0 {
		return fmt.Errorf("%w: an uninstall takes no revision", ErrInvalidName)
	}
	return nil
}
