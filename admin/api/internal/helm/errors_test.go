package helm

import (
	"errors"
	"strings"
	"testing"
)

// The Job log is the only place a failed rollout explains itself, so the API
// server's sentence has to survive translation.
//
// It did not: a self-upgrade whose whole log read "forbidden: upgrade release
// home-lab-admin" took an hour to trace to one namespace the Job held no
// RoleBinding in — which the refusal had named outright before this threw it
// away.
func TestForbiddenKeepsTheAPIServersSentence(t *testing.T) {
	// The shape Helm hands back: the API server's words wrapped in Helm's own,
	// which is why the untyped string check below it in translate exists at all.
	denied := errors.New(`could not get information about the resource ` +
		`RoleBinding "home-lab-admin-helm" in namespace "journal": rolebindings.rbac.` +
		`authorization.k8s.io "home-lab-admin-helm" is forbidden: User ` +
		`"system:serviceaccount:admin:admin-helm-job" cannot get resource "rolebindings"`)

	err := translate(denied, "upgrade release home-lab-admin")

	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
	for _, want := range []string{"upgrade release home-lab-admin", "rolebindings", "journal"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("want %q in the error, got %q", want, err.Error())
		}
	}
}
