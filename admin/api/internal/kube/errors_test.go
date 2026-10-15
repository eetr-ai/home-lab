package kube

import (
	"errors"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// A refusal has to name the act it refused.
//
// The panel drew "not permitted to read this" over every forbidden reply,
// including a create that the chart had never been given the grant for — so an
// operator went looking for a refused read that had not happened. This is the
// test that keeps the act attached to the error.
func TestForbiddenNamesTheActItRefused(t *testing.T) {
	denied := apierrors.NewForbidden(
		schema.GroupResource{Resource: "namespaces"}, "journal", errors.New("no"))

	err := translate(denied, "create namespace journal")

	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("want ErrForbidden, got %v", err)
	}
	if got := refusedAct(err); got != "create namespace journal" {
		t.Errorf("want the act, got %q", got)
	}
	// The API server's own sentence goes to the log, which is where the resource
	// and the account it names are worth having — and it must be in the string,
	// not merely reachable by unwrapping, because a logger prints the string.
	if !errors.Is(err, denied) {
		t.Error("want the API server's error kept underneath")
	}
	if !strings.Contains(err.Error(), "namespaces") {
		t.Errorf("want the API server's sentence in the error, got %q", err.Error())
	}
}

// ErrForbidden is also returned bare, by code that never went through translate.
// Those answers must still read as a sentence rather than trailing off.
func TestForbiddenWithoutAnActStillReads(t *testing.T) {
	if got := refusedAct(ErrForbidden); got != "do this" {
		t.Errorf("want a fallback act, got %q", got)
	}
}
