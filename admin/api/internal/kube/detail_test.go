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
