package helm

import (
	"errors"
	"strings"
	"testing"
)

// The encoder and the decoder run in different processes — the API builds the
// command line, the Job's pod reads it back — so a mismatch between them would
// otherwise only ever be discovered by a pod that refused to start. Round-tripping
// them in one test is the cheapest place to catch that.
func TestJobSpecRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		spec JobSpec
	}{
		{
			name: "a rollout",
			spec: JobSpec{Operation: OpRollout, DeploymentID: "d29b1e0a", Version: 7},
		},
		{
			name: "a rollout that undoes itself on failure",
			spec: JobSpec{
				Operation:         OpRollout,
				DeploymentID:      "d29b1e0a",
				Version:           7,
				RollbackOnFailure: true,
			},
		},
		{
			name: "a rollback",
			spec: JobSpec{Operation: OpRollback, Namespace: "lab", Release: "podinfo", Revision: 3},
		},
		{
			name: "an uninstall",
			spec: JobSpec{Operation: OpUninstall, Namespace: "lab", Release: "podinfo"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := test.spec.Args()
			// Args includes the subcommand, which main consumes before parsing.
			if args[0] != RunCommand {
				t.Fatalf("args[0] = %q, want %q", args[0], RunCommand)
			}

			got, err := parseJobArgs(args[1:])
			if err != nil {
				t.Fatalf("parseJobArgs(%v): %v", args[1:], err)
			}
			if got != test.spec {
				t.Errorf("round trip = %+v, want %+v", got, test.spec)
			}
		})
	}
}

// Every one of these is a way for the API and the runner to disagree, and each
// would otherwise be a pod doing something adjacent to what was asked.
func TestParseJobArgsRefusals(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no operation at all",
			args: nil,
			want: "an operation is required",
		},
		{
			name: "an operation this binary does not perform",
			args: []string{"install"},
			want: "not a Helm operation",
		},
		{
			name: "a rollout with no deployment",
			args: []string{OpRollout, "--version", "3"},
			want: "needs a deployment",
		},
		{
			name: "a rollout with no version",
			args: []string{OpRollout, "--deployment", "d29b1e0a"},
			want: "needs a declared version",
		},
		{
			// Version numbers count from 1, so 0 is "unset" arriving as if it were
			// a choice.
			name: "a rollout with version zero",
			args: []string{OpRollout, "--deployment", "d29b1e0a", "--version", "0"},
			want: "needs a declared version",
		},
		{
			name: "a rollback with no revision",
			args: []string{OpRollback, "--namespace", "lab", "--release", "podinfo"},
			want: "needs a revision",
		},
		{
			name: "a release name Helm would not accept",
			args: []string{OpUninstall, "--namespace", "lab", "--release", "Podinfo"},
			want: "release names must be",
		},
		{
			name: "a namespace Kubernetes would not accept",
			args: []string{OpUninstall, "--namespace", "Lab", "--release", "podinfo"},
			want: "namespace names must be",
		},
		{
			// Silently ignoring this is how a rollback becomes an operation on a
			// release nobody named.
			name: "a rollout addressed by release as well",
			args: []string{OpRollout, "--deployment", "d29b1e0a", "--version", "3",
				"--namespace", "lab", "--release", "podinfo"},
			want: "addressed by deployment, not by release",
		},
		{
			name: "an uninstall addressed by deployment as well",
			args: []string{OpUninstall, "--namespace", "lab", "--release", "podinfo",
				"--deployment", "d29b1e0a"},
			want: "addressed by release, not by deployment",
		},
		{
			name: "an uninstall carrying a revision",
			args: []string{OpUninstall, "--namespace", "lab", "--release", "podinfo",
				"--revision", "2"},
			want: "takes no revision",
		},
		{
			name: "a flag with no value",
			args: []string{OpRollout, "--deployment"},
			want: "needs a value",
		},
		{
			name: "a version that is not a number",
			args: []string{OpRollout, "--deployment", "d29b1e0a", "--version", "seven"},
			want: "must be a number",
		},
		{
			name: "an argument this binary does not know",
			args: []string{OpRollout, "--chart", "podinfo"},
			want: "not a known argument",
		},
		{
			// A bare word rather than a flag. Reading it as a flag name is what
			// produces the "not a known argument" message; the point is that it is
			// refused rather than skipped.
			name: "a stray positional argument",
			args: []string{OpUninstall, "lab"},
			want: "needs a value",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseJobArgs(test.args)
			if err == nil {
				t.Fatalf("parseJobArgs(%v) = nil error, want one", test.args)
			}
			if !errors.Is(err, ErrInvalidName) {
				t.Errorf("error = %v, want it to wrap ErrInvalidName", err)
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("error = %q, want it to mention %q", err, test.want)
			}
		})
	}
}
