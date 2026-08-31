// Package nsenrol enrols a namespace as a Helm target, and reports whether one
// still is.
//
// It is a shared package rather than part of a slice for the reason nspolicy is
// one: two slices need it and neither may import the other. The cluster slice
// creates namespaces and shows an operator whether each is set up; the Helm slice
// asks which namespaces it may work in.
//
// # What enrolment is
//
// Helm keeps a release in a Secret in the namespace it was installed into, so the
// panel needs a grant in each namespace it deploys to. That grant used to be a
// Role rendered by the chart, one per namespace named in values — and a Role
// needs a concrete metadata.namespace, which Helm only knows at render time. So
// adding a namespace meant reinstalling the chart, which is precisely what
// "create a namespace from the panel, then deploy into it" cannot survive.
//
// A RoleBinding that references a ClusterRole has no such problem. The rules live
// in one cluster-scoped object the chart owns and reviewers read once; enrolling a
// namespace is two namespaced bindings, which the panel can create at runtime.
//
// # What that costs, stated plainly
//
// The API can enrol any namespace that is not protected, and enrolling one is
// what makes its Secrets readable. So the reach is the same as the cluster-wide
// grant this replaced: every unprotected namespace, eventually.
//
// What differs is that it is not standing. Reaching a namespace takes a
// deliberate act that leaves two named objects behind, and a namespace nobody
// enrolled is one the panel cannot read. `kubectl get rolebindings -A` is the
// whole audit. That is worth having, and it is not a smaller grant.
//
// The one thing that bounds it is how the API is permitted to create those
// bindings: `bind` on the two ClusterRoles by name, and nothing else. Kubernetes
// refuses any other roleRef, so this cannot hand out a grant the chart did not
// write.
package nsenrol

// State is how completely a namespace is enrolled.
//
// Four values rather than a bool, because "not set up" and "set up wrong" call
// for different words in front of an operator and the second is the one that
// happens to a lab that has been upgraded. A binding left by an older chart has
// the right name and the wrong roleRef; roleRef is immutable, so nothing will
// ever fix it in place and it will keep failing deploys until something notices.
type State string

const (
	// StateEnrolled is every binding present and correct.
	StateEnrolled State = "enrolled"
	// StateMissing is none of them. The ordinary state of a namespace nobody has
	// enrolled, and not an error.
	StateMissing State = "missing"
	// StatePartial is some of them — a binding deleted by hand, or an enrolment
	// that failed halfway.
	StatePartial State = "partial"
	// StateWrong is a binding of the right name pointing somewhere else, or bound
	// to a different subject. Old data, and the reason repair exists.
	StateWrong State = "wrong"
	// StateUnknown is what a namespace reports when the role bindings could not be
	// read at all.
	//
	// It exists so that one failing read does not take a page with it. Enrolment
	// is shown beside a namespace listing, and a listing that refuses to render
	// because a secondary answer was unavailable is worse than one that says it
	// does not know.
	StateUnknown State = "unknown"
)

// Binding is one RoleBinding, reduced to what deciding correctness needs.
type Binding struct {
	Name string
	// RoleRef is the name of the ClusterRole it grants. The kind is checked
	// too — a RoleBinding may reference a namespaced Role of the same name, and
	// that would grant something else entirely.
	RoleRefKind string
	RoleRef     string
	// Subjects are the ServiceAccounts it grants to, as "namespace/name".
	Subjects []string
}
