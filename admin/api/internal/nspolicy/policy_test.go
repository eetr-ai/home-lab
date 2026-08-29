package nspolicy

import "testing"

func TestProtected(t *testing.T) {
	lab := Config{
		Own:       "admin",
		Protected: []string{"platform-system"},
		Managed:   []string{"apps", "platform-system"},
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
		Managed:   []string{"apps", "platform-system", "admin"},
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
			// The list alone cannot be seen from the object, so it is not enough.
			name:      "configured but not labelled",
			namespace: "apps",
			want:      false,
		},
		{
			// The label alone can be applied by anything that can label a
			// namespace, so it is not enough either.
			name:      "labelled but not configured",
			namespace: "other",
			labels:    managed,
			want:      false,
		},
		{
			// Protection wins over the managed list, in both directions, or a
			// typo in one values file is a Helm release in platform-system.
			name:      "protected by configuration, however it is labelled",
			namespace: "platform-system",
			labels:    managed,
			want:      false,
		},
		{
			name:      "the panel's own namespace, however it is labelled",
			namespace: "admin",
			labels:    managed,
			want:      false,
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
