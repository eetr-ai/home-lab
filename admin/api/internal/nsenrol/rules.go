package nsenrol

import (
	"fmt"
	"slices"
)

// Config names the objects this creates. All of it comes from the chart, through
// the API's own environment — nothing here is influenced by a request.
type Config struct {
	// Release is the Helm release name the chart was installed under, which is
	// what its ClusterRoles are named after.
	Release string
	// Namespace is where the panel runs, and therefore where both ServiceAccounts
	// live.
	Namespace string
	// APIAccount is the long-lived identity the API pods carry a token for. It
	// gets the read-and-write-Secrets grant.
	APIAccount string
	// JobAccount is the identity a Helm operation runs as. It gets the deploy
	// grant, and holds it only for the lifetime of one Job.
	JobAccount string
}

// Valid reports whether there is enough configuration to enrol anything.
//
// Checked once at startup rather than per request: a panel configured to manage
// namespaces and unable to name the objects it would create is a misconfiguration
// to say out loud, not a 500 to discover later.
func (c Config) Valid() bool {
	return c.Release != "" && c.Namespace != "" && c.APIAccount != "" && c.JobAccount != ""
}

// wanted is one binding this creates, and what makes it correct.
type wanted struct {
	name    string
	role    string
	subject string
}

// Wanted returns the bindings a fully enrolled namespace has.
//
// Two, and the split is the security story of the whole feature: the deploy grant
// belongs to the Job's account, which exists for one operation, and the API's
// long-lived credential gets only what reading a release and writing a credential
// needs. Binding both to the same account would quietly undo that.
func (c Config) wanted() []wanted {
	return []wanted{
		{
			name:    c.Release + "-helm",
			role:    c.Release + "-helm",
			subject: c.Namespace + "/" + c.JobAccount,
		},
		{
			name:    c.Release + "-secrets",
			role:    c.Release + "-secrets",
			subject: c.Namespace + "/" + c.APIAccount,
		},
	}
}

// Names returns the binding names, which is what a lister filters on.
func (c Config) Names() []string {
	names := make([]string, 0, 2)
	for _, one := range c.wanted() {
		names = append(names, one.name)
	}
	return names
}

// matches reports whether a live binding is the one wanted.
//
// The kind is part of it. A RoleBinding may reference a namespaced Role with the
// same name as the ClusterRole, and Kubernetes would accept that happily — it
// would grant whatever that Role happens to say, which is not this chart's
// decision. So a ClusterRole reference is required rather than assumed.
func (w wanted) matches(live Binding) bool {
	return live.RoleRefKind == "ClusterRole" &&
		live.RoleRef == w.role &&
		slices.Contains(live.Subjects, w.subject)
}

// Describe renders a state as the sentence the panel shows.
func Describe(state State) string {
	switch state {
	case StateEnrolled:
		return "set up for Helm"
	case StateMissing:
		return "not set up for Helm"
	case StatePartial:
		return "partly set up: some of the panel's role bindings are missing"
	case StateWrong:
		return "set up wrongly: a role binding grants something else"
	default:
		return fmt.Sprintf("in an unknown state (%s)", state)
	}
}
