package kube

import (
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// summarizeSecret is the only place a live Secret becomes something this API will
// send, so it is the only place a value could escape.
//
// Asserted by marshalling the result and searching it, not by reading named
// fields: the way this breaks is somebody adding a field to SecretSummary, and a
// test that checks the fields it knows about would go on passing.
func TestSummarizeSecretCarriesNoValue(t *testing.T) {
	// Values distinctive enough that finding one in the output cannot be a
	// coincidental match on the Secret's own name.
	const (
		password = "hunter2-do-not-serialise"
		username = "sentinel-username-value"
		token    = "sentinel-token-value"
	)
	immutable := true

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "octo-database",
			Namespace: "apps",
			Labels:    map[string]string{labelManagedBy: managedByValue},
		},
		Type:      corev1.SecretTypeOpaque,
		Immutable: &immutable,
		Data:      map[string][]byte{"password": []byte(password), "username": []byte(username)},
		// A Secret read back from the API server has its values folded into Data,
		// but one that has not been through a round trip still has these — and a
		// projection that only walked Data would under-report its keys while
		// happily carrying this map somewhere else.
		StringData: map[string]string{"token": token},
	}

	summary := summarizeSecret(secret)

	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, value := range []string{password, username, token} {
		if strings.Contains(string(encoded), value) {
			t.Errorf("the summary carried a value: %s", encoded)
		}
	}

	// Keys sorted, and every key reported once whichever map it came from.
	if got := strings.Join(summary.Keys, ","); got != "password,token,username" {
		t.Errorf("Keys = %v, want password, token, username sorted", summary.Keys)
	}
	if !summary.Immutable {
		t.Error("Immutable = false, want true")
	}
	if !summary.PanelManaged {
		t.Error("PanelManaged = false, want true — the label is there")
	}
}

// A key present in both maps is reported once. Kubernetes resolves the overlap by
// letting StringData win, and a duplicate in this list would show the panel two
// rows for one key.
func TestSummarizeSecretDoesNotDoubleCountAKey(t *testing.T) {
	secret := &corev1.Secret{
		Data:       map[string][]byte{"password": []byte("old")},
		StringData: map[string]string{"password": "new"},
	}
	if got := summarizeSecret(secret).Keys; len(got) != 1 || got[0] != "password" {
		t.Errorf("Keys = %v, want [password]", got)
	}
}

// A Secret with no data at all is a Secret with no keys, not a nil the panel has
// to guess about. An empty slice marshals to [], a nil to null.
func TestSummarizeSecretReportsNoKeysAsEmpty(t *testing.T) {
	encoded, err := json.Marshal(summarizeSecret(&corev1.Secret{}))
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(encoded), `"keys":[]`) {
		t.Errorf("keys = %s, want []", encoded)
	}
}
