package kube

import (
	"errors"
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func editableSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "apps", Name: "octo-database"},
		Type:       corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"username": []byte("octo"),
			"password": []byte("hunter2"),
		},
	}
}

// The keys reported are the written object's, so they include whatever else is
// on it — not a list worked out from the request.
func TestEditSecretKeysReturnsTheWrittenKeys(t *testing.T) {
	client := fake.NewClientset(editableSecret())
	repo := NewRepository(client, client, nil, false)

	keys, err := repo.EditSecretKeys(t.Context(), "apps", "octo-database", SecretEdit{
		Add:    map[string]string{"apikey": "new-key"},
		Remove: []string{"username"},
	})
	if err != nil {
		t.Fatalf("EditSecretKeys() error = %v", err)
	}
	if !slices.Equal(keys, []string{"apikey", "password"}) {
		t.Errorf("keys = %v, want [apikey password]", keys)
	}
}

// Kubernetes validates a Secret against its type, which this panel does not.
// Its refusal is the caller's to fix: a 400 naming the field, never a 500, and
// never the server's own message, which is not ours to vouch for.
func TestEditSecretKeysReportsAnInvalidResultAsTheCallersMistake(t *testing.T) {
	client := fake.NewClientset(editableSecret())
	client.PrependReactor("update", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewInvalid(
			schema.GroupKind{Kind: "Secret"}, "octo-database",
			field.ErrorList{field.Required(field.NewPath("data").Key("tls.key"), "upstream detail")})
	})
	repo := NewRepository(client, client, nil, false)

	_, err := repo.EditSecretKeys(t.Context(), "apps", "octo-database",
		SecretEdit{Remove: []string{"username"}})

	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("EditSecretKeys() error = %v, want ErrInvalidName", err)
	}
	if !strings.Contains(err.Error(), "data[tls.key]") {
		t.Errorf("error %q does not name the field", err)
	}
	if strings.Contains(err.Error(), "upstream detail") {
		t.Errorf("error %q carries the server's message", err)
	}
}
