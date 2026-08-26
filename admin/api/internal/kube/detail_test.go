package kube

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// A workload with no selector must claim no pods. The trap is that both label
// sentinels render as the empty string, and an empty LabelSelector on a List is
// read by the API server as "everything" — so a guard written against Empty()
// passes labels.Nothing() straight through and returns the whole namespace.
func TestSelectorOfRefusesToMatchEverything(t *testing.T) {
	tests := []struct {
		name     string
		selector *metav1.LabelSelector
	}{
		{name: "no selector at all", selector: nil},
		{name: "a selector with no requirements", selector: &metav1.LabelSelector{}},
		{
			name: "a malformed selector",
			selector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{Key: "app", Operator: "NotAnOperator"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := selectorOf(test.selector)
			// This is the assertion that matters: the callers gate on the rendered
			// string, so it has to be empty for every one of these.
			if got.String() != "" {
				t.Errorf("selectorOf() rendered %q; want an empty selector the callers refuse",
					got.String())
			}
			// And the sentinel is Nothing, not Everything — which Empty() cannot tell
			// apart, hence the note in selectorOf.
			if got.Matches(labels.Set{"app": "anything"}) {
				t.Error("selectorOf() produced a selector that matches a labelled pod")
			}
		})
	}
}

// The ordinary case still works: a real selector matches what it should.
func TestSelectorOfMatchesItsLabels(t *testing.T) {
	got := selectorOf(&metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}})

	if !got.Matches(labels.Set{"app": "api", "extra": "ignored"}) {
		t.Error("a matching pod was not matched")
	}
	if got.Matches(labels.Set{"app": "other"}) {
		t.Error("a pod of a different workload was matched")
	}
	if got.String() == "" {
		t.Error("a real selector rendered empty, which the callers treat as no match")
	}
}

// A Service is matched by evaluating its selector against the pods' real labels.
// The earlier version rebuilt an equality set from the workload's selector, which
// dropped a multi-value In and — worse — turned a one-value NotIn into an
// equality label, asserting the opposite of what it required.
func TestReachesEvaluatesAgainstRealPodLabels(t *testing.T) {
	pods := []labels.Set{
		{"app": "api", "tier": "web", "pod-template-hash": "abc123"},
		{"app": "api", "tier": "web", "pod-template-hash": "def456"},
	}

	tests := []struct {
		name     string
		selector map[string]string
		want     bool
	}{
		{name: "a selector the pods satisfy", selector: map[string]string{"app": "api"}, want: true},
		{name: "two labels, both present", selector: map[string]string{"app": "api", "tier": "web"}, want: true},
		{name: "a label the pods do not carry", selector: map[string]string{"app": "worker"}, want: false},
		{
			name:     "one label right and one wrong is not a match",
			selector: map[string]string{"app": "api", "tier": "batch"},
		},
		{
			// Mid-rollout the old and new pods differ by the template hash, and a
			// Service pinned to one of them still reaches the workload.
			name:     "matches only one of the pods",
			selector: map[string]string{"pod-template-hash": "def456"},
			want:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := reaches(labels.SelectorFromSet(test.selector), pods)
			if got != test.want {
				t.Errorf("reaches() = %t; want %t", got, test.want)
			}
		})
	}
}

// A workload with no pods has no labels to match against, so nothing reaches it.
// Matching an empty set would make every Service in the namespace appear to.
func TestReachesWithNoPods(t *testing.T) {
	if reaches(labels.SelectorFromSet(map[string]string{"app": "api"}), nil) {
		t.Error("a Service matched a workload that has no pods")
	}
}
