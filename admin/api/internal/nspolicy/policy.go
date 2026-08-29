// Package nspolicy decides which namespaces the panel may not touch.
//
// It is a shared package rather than part of a slice because two slices need the
// same answer and neither may import the other: the cluster slice refuses to
// delete a protected namespace, and the Helm slice refuses to install into one.
// Two copies of this rule would be two places to get it wrong, and only one of
// them would be read.
//
// Nothing here does any I/O or imports Kubernetes. It is given a name and the
// labels somebody else read, and it answers.
package nspolicy

import (
	"slices"
	"strings"
)

// The labels this policy reads off a live namespace.
const (
	// LabelProtected marks a namespace the panel may not delete or Helm-write.
	//
	// A live label rather than only configuration, so protecting something new is
	// a kubectl label away rather than a chart release. It can only add
	// protection: setting it to "false" removes nothing, because a policy an
	// attacker can switch off by editing the object it protects is not a policy.
	LabelProtected = "home-lab.example/protected"

	// LabelManaged marks a namespace Helm may write to. Necessary but not
	// sufficient — see Managed.
	LabelManaged = "home-lab.example/helm-managed"

	// labelTrue is how a label says yes. One spelling, so a rule cannot hold in
	// one place and not another because someone wrote "True".
	labelTrue = "true"
)

// systemNamespaces are Kubernetes' own. Not lab policy, and not configurable:
// there is no reading of "delete kube-system" that this panel should carry out.
var systemNamespaces = []string{"default", "kube-system", "kube-public", "kube-node-lease"}

// systemPrefix catches the rest of them. A cluster carries kube-* namespaces this
// repository has never heard of — kube-flannel, kube-ovn — and none of them is
// the lab's to delete.
const systemPrefix = "kube-"

// Config is what the lab knows that the built-in rules do not.
type Config struct {
	// Own is the namespace this process is running in, read from the downward
	// API. It is here rather than hardcoded because the chart can be installed
	// under any release name, and a panel that can delete its own namespace is a
	// panel that can delete itself.
	Own string
	// Protected are the lab's own additions, from chart values.
	Protected []string
	// Managed are the namespaces Helm may write to. Named here rather than in the
	// Helm slice because protection has to win over it, and the two rules have to
	// be decided together or a typo in one values file is a release in
	// platform-system.
	Managed []string
	// ManageEverything makes every unprotected namespace a Helm target, instead
	// of only those named in Managed.
	//
	// It exists because the panel can create namespaces and the per-namespace
	// Roles are rendered by the chart: without it, a namespace created from the
	// panel cannot be deployed to until somebody reinstalls the chart, which
	// breaks "create a namespace, then deploy into it" — the workflow the whole
	// feature is for. Protection is unaffected either way.
	ManageEverything bool
}

// Policy answers questions about a namespace.
type Policy struct {
	own        string
	protected  []string
	managed    []string
	everything bool
}

// New builds the policy. Empty configuration is valid and still protects the
// built-ins; those are not the lab's to switch off.
func New(config Config) Policy {
	return Policy{
		own:        config.Own,
		protected:  slices.Clone(config.Protected),
		managed:    slices.Clone(config.Managed),
		everything: config.ManageEverything,
	}
}

// Protected reports whether a namespace may not be DELETED, and why.
//
// The reason is returned rather than logged because the panel renders it: an
// operator looking at a namespace with no delete button deserves to be told which
// rule took it away, and "protected" alone does not say whether that is
// Kubernetes' doing or theirs.
//
// Deletion only. Reads are unaffected — platform-system still lists its
// workloads, pods, events, and logs exactly as before — and so are Helm writes,
// which ask DeployBlocked instead. The two questions used to be one, and merging
// them was wrong in one specific place: see DeployBlocked.
func (p Policy) Protected(namespace string, labels map[string]string) (bool, string) {
	if p.own != "" && namespace == p.own {
		return true, "the namespace the panel runs in"
	}
	return p.blocked(namespace, labels)
}

// DeployBlocked reports whether Helm may NOT write to a namespace, and why.
//
// The same rules as Protected with one exception: the panel's own namespace is
// deployable. That is not an oversight to be tidied up later — upgrading the
// panel itself from a pipeline is the reason this whole feature exists, and a
// policy that refused it would refuse the use case it was built for.
//
// The two questions are genuinely different and were wrong to be one. Deleting
// the namespace the panel runs in destroys the panel and everything in it, and
// nothing about deploying needs that. Writing a release into it is an upgrade.
// So `admin` stays undeletable and becomes deployable, and the asymmetry is the
// point rather than a gap.
//
// Everything else is unchanged. Kubernetes' own namespaces, anything the lab
// configured as protected, and anything carrying the label are all still refused.
func (p Policy) DeployBlocked(namespace string, labels map[string]string) (bool, string) {
	return p.blocked(namespace, labels)
}

// blocked is what both questions share.
func (p Policy) blocked(namespace string, labels map[string]string) (bool, string) {
	switch {
	case slices.Contains(systemNamespaces, namespace), strings.HasPrefix(namespace, systemPrefix):
		return true, "a Kubernetes system namespace"
	case slices.Contains(p.protected, namespace):
		return true, "protected by this lab's configuration"
	case labels[LabelProtected] == labelTrue:
		return true, "protected by the " + LabelProtected + " label"
	default:
		return false, ""
	}
}

// ManagedNamespaces returns the namespaces configuration names as Helm's, minus
// any that are protected, in the order they were configured.
//
// This is the list to enumerate. Finding the managed namespaces by reading every
// namespace in the cluster and checking its label would need a cluster-wide grant
// on Helm's release Secrets, which is exactly what this design refuses to hold.
//
// Protection is applied here and not left to the caller. Enumerating a namespace
// is not a passive act: reading its Helm releases means reading its Secrets, so a
// protected name surviving into this list is the leak, not a step towards one.
// The chart refuses to render a protected namespace into the managed list, but
// that list also arrives from an environment variable — and a policy that holds
// only when the chart wrote it is not a policy.
//
// Labels are not consulted, because there is no object to read one from. That is
// the same bound checkNamespace works under, and it is why this is still not the
// same question as Managed: a name here has passed the checks that can be made
// from configuration alone, so anything that writes must still ask.
func (p Policy) ManagedNamespaces() []string {
	managed := make([]string, 0, len(p.managed))
	for _, namespace := range p.managed {
		if blocked, _ := p.DeployBlocked(namespace, nil); blocked {
			continue
		}
		managed = append(managed, namespace)
	}
	return managed
}

// Managed reports whether Helm may install into a namespace.
//
// Three conditions, and all of them are needed. The namespace must not be
// deploy-blocked, which wins over everything else here. It must be named in
// configuration, which is the operator's decision and is reviewable in a values
// file. And it must carry the label, which is what makes the decision visible on
// the object itself.
//
// Both halves of the allowlist are required because each is weak alone: the label
// can be applied by anything that can label a namespace, and the configured list
// cannot be seen by someone looking at the cluster.
func (p Policy) Managed(namespace string, labels map[string]string) bool {
	if blocked, _ := p.DeployBlocked(namespace, labels); blocked {
		return false
	}
	if p.everything {
		return labels[LabelManaged] == labelTrue
	}
	return slices.Contains(p.managed, namespace) && labels[LabelManaged] == labelTrue
}

// ManagesEverything reports whether every unprotected namespace is a Helm target.
//
// Callers that enumerate managed namespaces have to ask, because in this mode
// there is no list to enumerate — the answer is "whatever the cluster has", and
// only the cluster knows.
func (p Policy) ManagesEverything() bool {
	return p.everything
}
