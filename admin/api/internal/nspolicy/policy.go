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
}

// Policy answers questions about a namespace.
type Policy struct {
	own       string
	protected []string
	managed   []string
}

// New builds the policy. Empty configuration is valid and still protects the
// built-ins; those are not the lab's to switch off.
func New(config Config) Policy {
	return Policy{
		own:       config.Own,
		protected: slices.Clone(config.Protected),
		managed:   slices.Clone(config.Managed),
	}
}

// Protected reports whether a namespace may not be deleted or Helm-written, and
// why.
//
// The reason is returned rather than logged because the panel renders it: an
// operator looking at a namespace with no delete button deserves to be told which
// rule took it away, and "protected" alone does not say whether that is
// Kubernetes' doing or theirs.
//
// Protection covers deletion and Helm writes, and nothing else. Reads are
// unaffected — platform-system still lists its workloads, pods, events, and logs
// exactly as before.
func (p Policy) Protected(namespace string, labels map[string]string) (bool, string) {
	switch {
	case slices.Contains(systemNamespaces, namespace), strings.HasPrefix(namespace, systemPrefix):
		return true, "a Kubernetes system namespace"
	case p.own != "" && namespace == p.own:
		return true, "the namespace the panel runs in"
	case slices.Contains(p.protected, namespace):
		return true, "protected by this lab's configuration"
	case labels[LabelProtected] == "true":
		return true, "protected by the " + LabelProtected + " label"
	default:
		return false, ""
	}
}

// Managed reports whether Helm may install into a namespace.
//
// Three conditions, and all of them are needed. The namespace must not be
// protected, which wins over everything else here. It must be named in
// configuration, which is the operator's decision and is reviewable in a values
// file. And it must carry the label, which is what makes the decision visible on
// the object itself.
//
// Both halves of the allowlist are required because each is weak alone: the label
// can be applied by anything that can label a namespace, and the configured list
// cannot be seen by someone looking at the cluster.
func (p Policy) Managed(namespace string, labels map[string]string) bool {
	if protected, _ := p.Protected(namespace, labels); protected {
		return false
	}
	return slices.Contains(p.managed, namespace) && labels[LabelManaged] == "true"
}
