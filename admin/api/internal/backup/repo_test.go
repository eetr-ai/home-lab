package backup

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

// The fake clientset accepts an update with no resourceVersion, and the real API
// server does not. So the reactor below refuses one the way the API server would,
// which is the only way this test can see the bug it exists for: every reconnect
// failing after Google had already spent the code.
func TestReplacingTheGrantCarriesTheResourceVersion(t *testing.T) {
	clientset := fake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "token", Namespace: "admin", ResourceVersion: "41"},
		Data:       map[string][]byte{refreshTokenKey: []byte("old")},
	})
	clientset.PrependReactor("update", "secrets",
		func(action clienttesting.Action) (bool, runtime.Object, error) {
			updated := action.(clienttesting.UpdateAction).GetObject().(*corev1.Secret)
			if updated.ResourceVersion != "41" {
				t.Errorf("the update must carry the existing resourceVersion, carried %q", updated.ResourceVersion)
			}
			return false, nil, nil
		})

	repo := &Repository{clientset: clientset, namespace: "admin", tokenSecret: "token"}
	if err := repo.WriteGrant(context.Background(), Grant{RefreshToken: "new", Account: "a"}); err != nil {
		t.Fatal(err)
	}

	grant, err := repo.ReadGrant(context.Background())
	if err != nil || grant.RefreshToken != "new" {
		t.Errorf("the grant should be replaced, got %+v, %v", grant, err)
	}
}

func TestTheFirstGrantIsCreated(t *testing.T) {
	repo := &Repository{clientset: fake.NewClientset(), namespace: "admin", tokenSecret: "token"}
	if err := repo.WriteGrant(context.Background(), Grant{RefreshToken: "first"}); err != nil {
		t.Fatal(err)
	}
	if grant, err := repo.ReadGrant(context.Background()); err != nil || grant.RefreshToken != "first" {
		t.Errorf("got %+v, %v", grant, err)
	}
}
