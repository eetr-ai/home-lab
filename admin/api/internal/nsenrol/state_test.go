package nsenrol

import (
	"slices"
	"testing"
)

var config = Config{
	Release:    "home-lab-admin",
	Namespace:  "admin",
	APIAccount: "admin-api",
	JobAccount: "admin-helm-job",
}

// The bindings a correctly enrolled namespace has.
func correct() []Binding {
	return []Binding{
		{
			Name: "home-lab-admin-helm", RoleRefKind: "ClusterRole", RoleRef: "home-lab-admin-helm",
			Subjects: []string{"admin/admin-helm-job"},
		},
		{
			Name: "home-lab-admin-secrets", RoleRefKind: "ClusterRole", RoleRef: "home-lab-admin-secrets",
			Subjects: []string{"admin/admin-api"},
		},
	}
}

// What a namespace's bindings mean, and what would fix them.
//
// The two that matter are the last two. A binding left by an older chart has the
// right name and the wrong roleRef, which is immutable — so nothing patches it
// into shape, it keeps failing deploys, and only a delete-and-recreate clears it.
// And a binding pointing at a namespaced Role rather than the ClusterRole is
// accepted by Kubernetes and grants whatever that Role says, which is not this
// chart's decision.
func TestDecide(t *testing.T) {
	tests := []struct {
		name        string
		live        []Binding
		wantState   State
		wantCreate  []string
		wantReplace []string
	}{
		{
			name:      "correct bindings need nothing",
			live:      correct(),
			wantState: StateEnrolled,
		},
		{
			name:       "no bindings is missing, and creates both",
			live:       nil,
			wantState:  StateMissing,
			wantCreate: []string{"home-lab-admin-helm", "home-lab-admin-secrets"},
		},
		{
			name:       "one binding is partial, and creates the other",
			live:       correct()[:1],
			wantState:  StatePartial,
			wantCreate: []string{"home-lab-admin-secrets"},
		},
		{
			name: "a binding pointing somewhere else is wrong, and is replaced",
			live: []Binding{
				{
					Name: "home-lab-admin-helm", RoleRefKind: "ClusterRole", RoleRef: "cluster-admin",
					Subjects: []string{"admin/admin-helm-job"},
				},
				correct()[1],
			},
			wantState:   StateWrong,
			wantCreate:  []string{"home-lab-admin-helm"},
			wantReplace: []string{"home-lab-admin-helm"},
		},
		{
			name: "a Role of the same name is not the ClusterRole",
			live: []Binding{
				{
					Name: "home-lab-admin-helm", RoleRefKind: "Role", RoleRef: "home-lab-admin-helm",
					Subjects: []string{"admin/admin-helm-job"},
				},
				correct()[1],
			},
			wantState:   StateWrong,
			wantCreate:  []string{"home-lab-admin-helm"},
			wantReplace: []string{"home-lab-admin-helm"},
		},
		{
			name: "a binding granting the wrong account is wrong",
			live: []Binding{
				{
					Name: "home-lab-admin-helm", RoleRefKind: "ClusterRole", RoleRef: "home-lab-admin-helm",
					Subjects: []string{"admin/admin-api"},
				},
				correct()[1],
			},
			wantState:   StateWrong,
			wantCreate:  []string{"home-lab-admin-helm"},
			wantReplace: []string{"home-lab-admin-helm"},
		},
		{
			// Wrong wins over partial: a binding that grants the wrong thing is
			// worse than one that is absent, and the word in front of the operator
			// should be the worse one.
			name:        "wrong wins over missing",
			live:        []Binding{{Name: "home-lab-admin-helm", RoleRefKind: "ClusterRole", RoleRef: "cluster-admin"}},
			wantState:   StateWrong,
			wantCreate:  []string{"home-lab-admin-helm", "home-lab-admin-secrets"},
			wantReplace: []string{"home-lab-admin-helm"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := config.Decide(test.live)

			if plan.State != test.wantState {
				t.Errorf("State = %q, want %q", plan.State, test.wantState)
			}
			created := names(plan.Create)
			if !slices.Equal(created, test.wantCreate) {
				t.Errorf("Create = %v, want %v", created, test.wantCreate)
			}
			if !slices.Equal(plan.Replace, test.wantReplace) {
				t.Errorf("Replace = %v, want %v", plan.Replace, test.wantReplace)
			}
			if plan.Done() != (test.wantState == StateEnrolled) {
				t.Errorf("Done() = %v for state %q", plan.Done(), plan.State)
			}
		})
	}
}

// Every binding this creates references a ClusterRole, and the two accounts are
// not interchangeable: the deploy grant belongs to the Job's account, which lives
// for one operation, and binding it to the API's long-lived credential would
// quietly undo the whole split.
func TestCreatedBindingsGrantTheRightAccounts(t *testing.T) {
	plan := config.Decide(nil)

	want := map[string]string{
		"home-lab-admin-helm":    "admin/admin-helm-job",
		"home-lab-admin-secrets": "admin/admin-api",
	}
	for _, binding := range plan.Create {
		if binding.RoleRefKind != "ClusterRole" {
			t.Errorf("%s references a %s", binding.Name, binding.RoleRefKind)
		}
		if !slices.Equal(binding.Subjects, []string{want[binding.Name]}) {
			t.Errorf("%s grants %v, want %v", binding.Name, binding.Subjects, want[binding.Name])
		}
	}
}

func TestConfigValid(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   bool
	}{
		{name: "complete", config: config, want: true},
		{name: "no release", config: Config{Namespace: "admin", APIAccount: "a", JobAccount: "b"}},
		{name: "no namespace", config: Config{Release: "r", APIAccount: "a", JobAccount: "b"}},
		{name: "no api account", config: Config{Release: "r", Namespace: "admin", JobAccount: "b"}},
		{name: "no job account", config: Config{Release: "r", Namespace: "admin", APIAccount: "a"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.config.Valid(); got != test.want {
				t.Errorf("Valid() = %v, want %v", got, test.want)
			}
		})
	}
}

func names(bindings []Binding) []string {
	out := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, binding.Name)
	}
	return out
}
