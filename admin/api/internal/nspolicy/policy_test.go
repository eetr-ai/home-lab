package nspolicy

import (
	"testing"
)

func TestProtected(t *testing.T) {
	lab := Config{
		Own:       "admin",
		Protected: []string{"platform-system"},
	}

	tests := []struct {
		name      string
		config    Config
		namespace string
		labels    map[string]string
		want      bool
	}{
		{
			name:      "a Kubernetes system namespace",
			config:    lab,
			namespace: "kube-system",
			want:      true,
		},
		{
			// The prefix, not the list: a cluster may carry kube-something this
			// repository has never heard of, and none of them is the lab's.
			name:      "anything under the kube- prefix",
			config:    lab,
			namespace: "kube-flannel",
			want:      true,
		},
		{
			name:      "the default namespace",
			config:    lab,
			namespace: "default",
			want:      true,
		},
		{
			// The panel deleting the namespace it runs in is the failure this
			// exists to make impossible.
			name:      "the namespace the panel is running in",
			config:    lab,
			namespace: "admin",
			want:      true,
		},
		{
			name:      "a namespace named in configuration",
			config:    lab,
			namespace: "platform-system",
			want:      true,
		},
		{
			name:      "a namespace carrying the protected label",
			config:    lab,
			namespace: "apps",
			labels:    map[string]string{LabelProtected: "true"},
			want:      true,
		},
		{
			// The label adds protection and cannot remove it. A policy an
			// attacker can switch off by editing the object it protects is not a
			// policy.
			name:      "the label set to false does not unprotect a built-in",
			config:    lab,
			namespace: "kube-system",
			labels:    map[string]string{LabelProtected: "false"},
			want:      true,
		},
		{
			name:      "the label set to false does not unprotect a configured namespace",
			config:    lab,
			namespace: "platform-system",
			labels:    map[string]string{LabelProtected: "false"},
			want:      true,
		},
		{
			name:      "an ordinary namespace",
			config:    lab,
			namespace: "apps",
			want:      false,
		},
		{
			// Configuration can be empty; the built-ins are not configuration.
			name:      "the built-ins hold with no configuration at all",
			config:    Config{},
			namespace: "kube-system",
			want:      true,
		},
		{
			name:      "an unconfigured lab protects nothing else",
			config:    Config{},
			namespace: "apps",
			want:      false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			protected, reason := New(test.config).Protected(test.namespace, test.labels)
			if protected != test.want {
				t.Errorf("Protected(%q) = %v, want %v", test.namespace, protected, test.want)
			}
			if protected && reason == "" {
				t.Error("a protected namespace was given no reason; the panel renders it")
			}
			if !protected && reason != "" {
				t.Errorf("an unprotected namespace was given the reason %q", reason)
			}
		})
	}
}

func TestManaged(t *testing.T) {
	lab := Config{
		Own:       "admin",
		Protected: []string{"platform-system"},
	}
	managed := map[string]string{LabelManaged: "true"}

	tests := []struct {
		name      string
		namespace string
		labels    map[string]string
		want      bool
	}{
		{
			name:      "configured and labelled",
			namespace: "apps",
			labels:    managed,
			want:      true,
		},
		{
			// The label is the whole of the question here. A namespace that does
			// not carry it has not asked to be a Helm target — and the role
			// bindings, which are the other half, are nsenrol's answer rather than
			// this one's.
			name:      "not labelled",
			namespace: "apps",
			want:      false,
		},
		{
			name:      "any labelled namespace, not only a configured one",
			namespace: "other",
			labels:    managed,
			want:      true,
		},
		{
			// Protection wins, however the namespace is labelled, or one label
			// applied by hand is a Helm release in platform-system.
			name:      "protected by configuration, however it is labelled",
			namespace: "platform-system",
			labels:    managed,
			want:      false,
		},
		{
			// Deployable, and deliberately so: upgrading the panel itself from a
			// pipeline is the reason this feature exists. It is still not
			// deletable — see TestTheOwnNamespaceIsUndeletableButDeployable.
			name:      "the panel's own namespace, labelled managed",
			namespace: "admin",
			labels:    managed,
			want:      true,
		},
		{
			name:      "labelled managed and also labelled protected",
			namespace: "apps",
			labels:    map[string]string{LabelManaged: "true", LabelProtected: "true"},
			want:      false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := New(lab).Managed(test.namespace, test.labels); got != test.want {
				t.Errorf("Managed(%q, %v) = %v, want %v", test.namespace, test.labels, got, test.want)
			}
		})
	}
}

// The asymmetry the whole feature turns on, pinned in one place.
//
// The panel's own namespace may not be deleted — that destroys the panel and
// everything in it, and nothing about deploying needs it — and may be deployed
// into, because upgrading the panel from a pipeline is what this was built for.
// Merging the two questions, which is what the first version of this policy did,
// refuses the use case.
func TestTheOwnNamespaceIsUndeletableButDeployable(t *testing.T) {
	policy := New(Config{Own: "admin", Protected: []string{"platform-system"}})

	protected, reason := policy.Protected("admin", nil)
	if !protected {
		t.Error("the panel's own namespace must not be deletable")
	}
	if reason == "" {
		t.Error("a refusal has to say which rule took the delete button away")
	}

	if blocked, _ := policy.DeployBlocked("admin", nil); blocked {
		t.Error("the panel's own namespace must be deployable")
	}

	// And nothing else moved. Each of these is refused by both questions.
	for _, namespace := range []string{"platform-system", "kube-system", "kube-public", "default"} {
		if protected, _ := policy.Protected(namespace, nil); !protected {
			t.Errorf("%s must not be deletable", namespace)
		}
		if blocked, _ := policy.DeployBlocked(namespace, nil); !blocked {
			t.Errorf("%s must not be deployable", namespace)
		}
	}

	// The label still blocks both, including on the panel's own namespace: a
	// label can only ever add protection.
	labelled := map[string]string{LabelProtected: "true"}
	if blocked, _ := policy.DeployBlocked("admin", labelled); !blocked {
		t.Error("the protected label must still block a deploy into the own namespace")
	}
}
