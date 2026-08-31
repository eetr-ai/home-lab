package nsenrol

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/eetr-ai/home-lab/admin/api/internal/nspolicy"
)

// fakeRepo answers with fixed bindings and records the writes, so a test can
// assert that a refusal happened before anything reached the cluster.
type fakeRepo struct {
	bindings   map[string][]Binding
	candidates []Candidate
	created    []string
	deleted    []string
}

func (f *fakeRepo) ListBindings(_ context.Context, namespace string, names []string) ([]Binding, error) {
	kept := make([]Binding, 0, len(names))
	for _, binding := range f.bindings[namespace] {
		if slices.Contains(names, binding.Name) {
			kept = append(kept, binding)
		}
	}
	return kept, nil
}

func (f *fakeRepo) ListAllBindings(_ context.Context, names []string) (map[string][]Binding, error) {
	grouped := map[string][]Binding{}
	for namespace, bindings := range f.bindings {
		for _, binding := range bindings {
			if slices.Contains(names, binding.Name) {
				grouped[namespace] = append(grouped[namespace], binding)
			}
		}
	}
	return grouped, nil
}

func (f *fakeRepo) CreateBinding(_ context.Context, namespace string, binding Binding) error {
	f.created = append(f.created, namespace+"/"+binding.Name)
	f.bindings[namespace] = append(f.bindings[namespace], binding)
	return nil
}

func (f *fakeRepo) DeleteBinding(_ context.Context, namespace, name string) error {
	f.deleted = append(f.deleted, namespace+"/"+name)
	kept := f.bindings[namespace][:0]
	for _, binding := range f.bindings[namespace] {
		if binding.Name != name {
			kept = append(kept, binding)
		}
	}
	f.bindings[namespace] = kept
	return nil
}

func (f *fakeRepo) ListCandidates(context.Context) ([]Candidate, error) {
	return f.candidates, nil
}

func newService(repo *fakeRepo) *Service {
	return NewService(repo, config, nspolicy.New(nspolicy.Config{
		Own:       "admin",
		Protected: []string{"platform-system"},
	}))
}

// Reconciling is both "set it up" and "repair it", so it has to be safe to press
// twice — and it must not write to a namespace that is already correct.
func TestReconcileIsIdempotent(t *testing.T) {
	repo := &fakeRepo{bindings: map[string][]Binding{}}
	service := newService(repo)
	labels := map[string]string{nspolicy.LabelManaged: "true"}

	if _, err := service.Reconcile(t.Context(), "apps", labels); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(repo.created) != 2 {
		t.Fatalf("first Reconcile created %v, want two bindings", repo.created)
	}

	repo.created = nil
	state, err := service.Reconcile(t.Context(), "apps", labels)
	if err != nil {
		t.Fatalf("second Reconcile() error = %v", err)
	}
	if state != StateEnrolled {
		t.Errorf("state = %q, want %q", state, StateEnrolled)
	}
	if len(repo.created) != 0 {
		t.Errorf("second Reconcile wrote %v to a namespace that was already correct", repo.created)
	}
}

// The repair path, and the one this feature exists for: a binding left by an
// older chart points somewhere else, roleRef is immutable, and the only thing
// that clears it is a delete followed by a create.
func TestReconcileReplacesAWrongBinding(t *testing.T) {
	repo := &fakeRepo{bindings: map[string][]Binding{
		"apps": {
			{
				Name: "home-lab-admin-helm", RoleRefKind: "ClusterRole", RoleRef: "something-older",
				Subjects: []string{"admin/admin-helm-job"},
			},
		},
	}}
	service := newService(repo)

	if _, err := service.Reconcile(t.Context(), "apps",
		map[string]string{nspolicy.LabelManaged: "true"}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if !slices.Equal(repo.deleted, []string{"apps/home-lab-admin-helm"}) {
		t.Errorf("deleted = %v, want the wrong binding", repo.deleted)
	}
	if !slices.Contains(repo.created, "apps/home-lab-admin-helm") {
		t.Errorf("created = %v, want the binding recreated", repo.created)
	}
}

// Protection wins over enrolment, and it is checked before anything is written.
// A namespace holding credentials that matter is not one to make readable by
// pressing a button.
func TestReconcileRefusesAProtectedNamespace(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		labels    map[string]string
	}{
		{name: "the lab's own list", namespace: "platform-system"},
		{name: "a Kubernetes namespace", namespace: "kube-system"},
		{
			name: "the protected label", namespace: "labelled",
			labels: map[string]string{nspolicy.LabelProtected: "true"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepo{bindings: map[string][]Binding{}}
			_, err := newService(repo).Reconcile(t.Context(), test.namespace, test.labels)

			if !errors.Is(err, ErrProtected) {
				t.Fatalf("Reconcile() error = %v, want %v", err, ErrProtected)
			}
			if len(repo.created) != 0 || len(repo.deleted) != 0 {
				t.Errorf("a refused request wrote: created=%v deleted=%v", repo.created, repo.deleted)
			}
		})
	}
}

// The panel's own namespace is deployable, because upgrading the panel from a
// pipeline is what the whole Helm feature was asked for. Undeletable is a
// different question, decided elsewhere.
func TestReconcileEnrolsThePanelsOwnNamespace(t *testing.T) {
	repo := &fakeRepo{bindings: map[string][]Binding{}}
	if _, err := newService(repo).Reconcile(t.Context(), "admin",
		map[string]string{nspolicy.LabelManaged: "true"}); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if len(repo.created) != 2 {
		t.Errorf("created = %v, want the panel's own namespace enrolled", repo.created)
	}
}

// Managed is what the Helm slice enumerates, and a namespace only counts when
// the bindings are actually there: enumerating one means reading its Secrets, so
// answering with a namespace that has no binding would turn a listing into a 403.
func TestManagedNeedsTheLabelPolicyAndTheBindings(t *testing.T) {
	managed := map[string]string{nspolicy.LabelManaged: "true"}
	repo := &fakeRepo{
		candidates: []Candidate{
			{Name: "apps", Labels: managed},
			{Name: "half", Labels: managed},
			{Name: "bare", Labels: managed},
			{Name: "platform-system", Labels: managed},
		},
		bindings: map[string][]Binding{
			"apps":            correct(),
			"half":            correct()[:1],
			"platform-system": correct(),
		},
	}

	got, err := newService(repo).Managed(t.Context())
	if err != nil {
		t.Fatalf("Managed() error = %v", err)
	}
	if !slices.Equal(got, []string{"apps"}) {
		t.Errorf("Managed() = %v, want [apps]", got)
	}
}

// A namespace that has not asked to be a Helm target reports missing rather than
// anything that would invite a repair button.
func TestStateIgnoresANamespaceWithoutTheLabel(t *testing.T) {
	repo := &fakeRepo{bindings: map[string][]Binding{"apps": correct()}}
	state, err := newService(repo).State(t.Context(), "apps", nil)
	if err != nil {
		t.Fatalf("State() error = %v", err)
	}
	if state != StateMissing {
		t.Errorf("State() = %q, want %q", state, StateMissing)
	}
}

// Revoking removes the panel's bindings and nothing else. It is what makes
// enrolment reversible, which is what makes it safe to offer.
func TestRevokeRemovesBothBindings(t *testing.T) {
	repo := &fakeRepo{bindings: map[string][]Binding{"apps": correct()}}
	if err := newService(repo).Revoke(t.Context(), "apps"); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	if len(repo.deleted) != 2 {
		t.Errorf("deleted = %v, want both bindings", repo.deleted)
	}
}

// A listing reports enrolment for the namespaces that asked to be Helm targets,
// and says nothing about the ones that did not.
func TestStatesCoversOnlyTheCandidates(t *testing.T) {
	managed := map[string]string{nspolicy.LabelManaged: "true"}
	repo := &fakeRepo{bindings: map[string][]Binding{
		"apps": correct(),
		"half": correct()[:1],
	}}

	states, err := newService(repo).States(t.Context(), []Candidate{
		{Name: "apps", Labels: managed},
		{Name: "half", Labels: managed},
		{Name: "bare", Labels: managed},
		{Name: "unlabelled"},
		{Name: "platform-system", Labels: managed},
	})
	if err != nil {
		t.Fatalf("States() error = %v", err)
	}

	want := map[string]State{"apps": StateEnrolled, "half": StatePartial, "bare": StateMissing}
	if len(states) != len(want) {
		t.Fatalf("States() = %v, want %v", states, want)
	}
	for name, state := range want {
		if states[name] != state {
			t.Errorf("States()[%q] = %q, want %q", name, states[name], state)
		}
	}
}
