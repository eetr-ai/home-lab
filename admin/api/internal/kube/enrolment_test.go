package kube

import (
	"context"
	"errors"
	"testing"

	"github.com/eetr-ai/home-lab/admin/api/internal/nsenrol"
	"github.com/eetr-ai/home-lab/admin/api/internal/nspolicy"
)

// fakeEnrolment answers with fixed states and records what it was asked to
// reconcile, so a test can tell "the namespace was created and enrolled" from
// "the namespace was created".
type fakeEnrolment struct {
	states     map[string]nsenrol.State
	reconciled []string
	revoked    []string
	err        error
	statesErr  error
}

func (f *fakeEnrolment) States(_ context.Context, namespaces []nsenrol.Candidate) (map[string]nsenrol.State, error) {
	if f.statesErr != nil {
		return nil, f.statesErr
	}
	states := map[string]nsenrol.State{}
	for _, namespace := range namespaces {
		if state, ok := f.states[namespace.Name]; ok {
			states[namespace.Name] = state
		}
	}
	return states, nil
}

func (f *fakeEnrolment) State(_ context.Context, namespace string, _ map[string]string) (nsenrol.State, error) {
	return f.states[namespace], f.err
}

func (f *fakeEnrolment) Reconcile(_ context.Context, namespace string, _ map[string]string) (nsenrol.State, error) {
	if f.err != nil {
		return "", f.err
	}
	f.reconciled = append(f.reconciled, namespace)
	return nsenrol.StateEnrolled, nil
}

func (f *fakeEnrolment) Revoke(_ context.Context, namespace string) error {
	if f.err != nil {
		return f.err
	}
	f.revoked = append(f.revoked, namespace)
	return nil
}

func serviceWithEnrolment(repo repository, enrol Enrolment) *Service {
	service, err := NewService(repo, nspolicy.New(nspolicy.Config{
		Own:       "admin",
		Protected: []string{"platform-system"},
	}), "", enrol)
	if err != nil {
		panic(err)
	}
	return service
}

// A namespace created from the panel is enrolled on the spot, because "create a
// namespace, then deploy into it" is the workflow this exists for and a
// namespace that needs a second step is one somebody forgets to take.
func TestCreateNamespaceEnrolsIt(t *testing.T) {
	enrol := &fakeEnrolment{states: map[string]nsenrol.State{}}
	namespace, err := serviceWithEnrolment(&fakeRepo{}, enrol).
		CreateNamespace(t.Context(), NamespaceSpec{Name: "octo"})
	if err != nil {
		t.Fatalf("CreateNamespace() error = %v", err)
	}

	if len(enrol.reconciled) != 1 || enrol.reconciled[0] != "octo" {
		t.Errorf("reconciled = %v, want [octo]", enrol.reconciled)
	}
	if namespace.HelmEnrolment != string(nsenrol.StateEnrolled) {
		t.Errorf("HelmEnrolment = %q, want %q", namespace.HelmEnrolment, nsenrol.StateEnrolled)
	}
}

// A failed enrolment does not undo the namespace. It exists and it is correct;
// what is missing is two role bindings the repair action creates on demand, and
// deleting a namespace to tidy up after a failed second step would be worse than
// anything it fixed.
func TestCreateNamespaceKeepsTheNamespaceWhenEnrolmentFails(t *testing.T) {
	repo := &fakeRepo{}
	enrol := &fakeEnrolment{
		states: map[string]nsenrol.State{"octo": nsenrol.StateMissing},
		err:    errors.New("the api server said no"),
	}

	namespace, err := serviceWithEnrolment(repo, enrol).
		CreateNamespace(t.Context(), NamespaceSpec{Name: "octo"})
	if err != nil {
		t.Fatalf("CreateNamespace() error = %v, want the namespace to survive", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("created = %v, want the namespace created", repo.created)
	}
	// The state it is really in, not silence. "missing" is what puts the repair
	// action in front of the operator; an empty string renders as a namespace
	// nobody ever asked to be a Helm target, which is the one thing it is not.
	if namespace.HelmEnrolment != string(nsenrol.StateMissing) {
		t.Errorf("HelmEnrolment = %q, want %q", namespace.HelmEnrolment, nsenrol.StateMissing)
	}
}

// Enrolment is reported beside every namespace, so an operator can see which
// ones are set up without opening each.
func TestListNamespacesCarriesEnrolment(t *testing.T) {
	enrol := &fakeEnrolment{states: map[string]nsenrol.State{"default": nsenrol.StatePartial}}
	namespaces, err := serviceWithEnrolment(&fakeRepo{}, enrol).ListNamespaces(t.Context())
	if err != nil {
		t.Fatalf("ListNamespaces() error = %v", err)
	}
	if namespaces[0].HelmEnrolment != string(nsenrol.StatePartial) {
		t.Errorf("HelmEnrolment = %q, want %q", namespaces[0].HelmEnrolment, nsenrol.StatePartial)
	}
}

// One failing read must not take the page with it. The namespaces are what the
// panel exists to show; enrolment is a second answer beside them, and "unknown"
// is a better page than none.
func TestListNamespacesSurvivesAFailedEnrolmentRead(t *testing.T) {
	enrol := &fakeEnrolment{statesErr: errors.New("forbidden")}
	repo := &fakeRepo{}

	namespaces, err := serviceWithEnrolment(repo, enrol).ListNamespaces(t.Context())
	if err != nil {
		t.Fatalf("ListNamespaces() error = %v, want the listing to survive", err)
	}
	if len(namespaces) != 1 {
		t.Fatalf("namespaces = %v, want the cluster still listed", namespaces)
	}
}

// Without enrolment configured the routes answer 501 rather than pretending the
// request was malformed: the capability is built and this lab has not switched
// it on.
func TestEnrolmentRoutesRefuseAnUnconfiguredPanel(t *testing.T) {
	service := serviceWithEnrolment(&fakeRepo{}, nil)

	if _, err := service.EnrolNamespace(t.Context(), "octo"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("EnrolNamespace() error = %v, want %v", err, ErrNotConfigured)
	}
	if err := service.RevokeNamespace(t.Context(), "octo"); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("RevokeNamespace() error = %v, want %v", err, ErrNotConfigured)
	}
}

// A malformed name is refused before the cluster is asked, the same as every
// other namespace route.
func TestEnrolmentValidatesTheName(t *testing.T) {
	enrol := &fakeEnrolment{states: map[string]nsenrol.State{}}
	repo := &fakeRepo{}

	if _, err := serviceWithEnrolment(repo, enrol).
		EnrolNamespace(t.Context(), "Not A Namespace"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("EnrolNamespace() error = %v, want %v", err, ErrInvalidName)
	}
	if len(enrol.reconciled) != 0 {
		t.Errorf("a refused request reconciled %v", enrol.reconciled)
	}
}

// Enrolling and repairing are the same request, and both read the live namespace
// first: the labels are what the decision turns on, and a label is exactly what
// can have changed since the listing the operator is looking at was rendered.
func TestEnrolNamespaceReadsTheLiveNamespace(t *testing.T) {
	repo := &fakeRepo{namespaces: map[string]Namespace{
		"octo": {Name: "octo", Labels: map[string]string{nspolicy.LabelManaged: "true"}},
	}}
	enrol := &fakeEnrolment{states: map[string]nsenrol.State{"octo": nsenrol.StateWrong}}

	namespace, err := serviceWithEnrolment(repo, enrol).EnrolNamespace(t.Context(), "octo")
	if err != nil {
		t.Fatalf("EnrolNamespace() error = %v", err)
	}
	if len(enrol.reconciled) != 1 || enrol.reconciled[0] != "octo" {
		t.Errorf("reconciled = %v, want [octo]", enrol.reconciled)
	}
	if namespace.HelmEnrolment != string(nsenrol.StateEnrolled) {
		t.Errorf("HelmEnrolment = %q, want the state it ended in", namespace.HelmEnrolment)
	}
}

// Revoking is what makes enrolment reversible, which is what makes it safe to
// offer at all.
func TestRevokeNamespaceRemovesTheBindings(t *testing.T) {
	repo := &fakeRepo{namespaces: map[string]Namespace{"octo": {Name: "octo"}}}
	enrol := &fakeEnrolment{states: map[string]nsenrol.State{}}

	if err := serviceWithEnrolment(repo, enrol).RevokeNamespace(t.Context(), "octo"); err != nil {
		t.Fatalf("RevokeNamespace() error = %v", err)
	}
	if len(enrol.revoked) != 1 || enrol.revoked[0] != "octo" {
		t.Errorf("revoked = %v, want [octo]", enrol.revoked)
	}
}

// A namespace that is not there is a 404 rather than an enrolment of nothing.
func TestEnrolNamespaceRefusesAMissingNamespace(t *testing.T) {
	enrol := &fakeEnrolment{states: map[string]nsenrol.State{}}
	_, err := serviceWithEnrolment(&fakeRepo{}, enrol).EnrolNamespace(t.Context(), "gone")

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("EnrolNamespace() error = %v, want %v", err, ErrNotFound)
	}
	if len(enrol.reconciled) != 0 {
		t.Errorf("a missing namespace was reconciled: %v", enrol.reconciled)
	}
}

// Reading one namespace carries its enrolment, the same as the listing does.
//
// This is a regression test with a live failure behind it: applyEnrolment
// decorates the slice it is given, and passing it a one-element literal built
// from a local variable decorated a copy — so every single-namespace read
// reported no enrolment while the listing beside it was correct.
func TestReadNamespaceCarriesEnrolment(t *testing.T) {
	repo := &fakeRepo{namespaces: map[string]Namespace{
		"octo": {Name: "octo", Labels: map[string]string{nspolicy.LabelManaged: "true"}},
	}}
	enrol := &fakeEnrolment{states: map[string]nsenrol.State{"octo": nsenrol.StatePartial}}

	namespace, err := serviceWithEnrolment(repo, enrol).ReadNamespace(t.Context(), "octo")
	if err != nil {
		t.Fatalf("ReadNamespace() error = %v", err)
	}
	if namespace.HelmEnrolment != string(nsenrol.StatePartial) {
		t.Errorf("HelmEnrolment = %q, want %q", namespace.HelmEnrolment, nsenrol.StatePartial)
	}
}
