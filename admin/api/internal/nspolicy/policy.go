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
}

// Policy answers questions about a namespace.
type Policy struct {
	own       string
	protected []string
}

// New builds the policy. Empty configuration is valid and still protects the
// built-ins; those are not the lab's to switch off.
func New(config Config) Policy {
	return Policy{
		own:       config.Own,
		protected: slices.Clone(config.Protected),
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

// Managed reports whether Helm may work in a namespace.
//
// Two conditions. The namespace must not be deploy-blocked, which wins over
// everything else here; and it must carry the label, which is what makes the
// decision visible on the object itself.
//
// There used to be a third: a list of namespaces in chart values, which had to
// agree with the label before anything was managed. It is gone, and what replaced
// it is not "one key instead of two" — it is a second key that is also a live
// object. Nothing can actually be deployed into a namespace whose RoleBindings
// the panel has not created, and those are as visible in the cluster as the label
// is. The list had to go because it was rendered at install time: enrolling a
// namespace meant a chart release and a pod restart, which is exactly the thing
// "create a namespace, then deploy into it" cannot survive.
//
// Labels are read off the live object, so this answers for a namespace somebody
// labelled by hand a moment ago, with nothing to reinstall.
func (p Policy) Managed(namespace string, labels map[string]string) bool {
	if blocked, _ := p.DeployBlocked(namespace, labels); blocked {
		return false
	}
	return labels[LabelManaged] == labelTrue
}
