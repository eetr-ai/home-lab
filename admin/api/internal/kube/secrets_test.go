package kube

import (
	"errors"
	"maps"
	"slices"
	"testing"
)

// The rules PutSecret enforces, and the one property that matters most: a
// refused request must not have written anything.
//
// The keys and the protection check are where the mistakes are. A key Kubernetes
// would refuse is a 500 from the API server rather than a 400 naming it; an empty
// value is a Secret that exists and does not work, which is worse than none; and
// a protected namespace is the one place a write must not reach at all.
func TestPutSecretRefusesBeforeItWrites(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		spec      SecretSpec
		wantErr   error
	}{
		{
			name:      "a valid Secret is written",
			namespace: "apps",
			spec:      SecretSpec{Name: "octo-database", Data: map[string]string{"password": "hunter2"}},
		},
		{
			name:      "a malformed namespace is refused",
			namespace: "Apps",
			spec:      SecretSpec{Name: "octo-database", Data: map[string]string{"password": "hunter2"}},
			wantErr:   ErrInvalidName,
		},
		{
			name:      "a malformed Secret name is refused",
			namespace: "apps",
			spec:      SecretSpec{Name: "Octo Database", Data: map[string]string{"password": "hunter2"}},
			wantErr:   ErrInvalidName,
		},
		{
			name:      "a key Kubernetes would not accept is refused",
			namespace: "apps",
			spec:      SecretSpec{Name: "octo-database", Data: map[string]string{"pass word": "hunter2"}},
			wantErr:   ErrInvalidName,
		},
		{
			// A Secret whose password key holds "" starts the workload and fails
			// to authenticate, and nothing about that says it was never written.
			name:      "an empty value is refused",
			namespace: "apps",
			spec:      SecretSpec{Name: "octo-database", Data: map[string]string{"password": ""}},
			wantErr:   ErrInvalidName,
		},
		{
			name:      "an empty payload is refused",
			namespace: "apps",
			spec:      SecretSpec{Name: "octo-database", Data: map[string]string{}},
			wantErr:   ErrInvalidName,
		},
		{
			name:      "a protected namespace is refused",
			namespace: "platform-system",
			spec:      SecretSpec{Name: "octo-database", Data: map[string]string{"password": "hunter2"}},
			wantErr:   ErrProtected,
		},
		{
			// The panel's own namespace holds its OIDC client secret and its
			// database connection strings. Deployable it may be; writable by this
			// route it is not.
			name:      "the panel's own namespace is refused",
			namespace: "admin",
			spec:      SecretSpec{Name: "octo-database", Data: map[string]string{"password": "hunter2"}},
			wantErr:   ErrProtected,
		},
		{
			name:      "a namespace carrying the protected label is refused",
			namespace: "labelled",
			spec:      SecretSpec{Name: "octo-database", Data: map[string]string{"password": "hunter2"}},
			wantErr:   ErrProtected,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepo{namespaces: namespacesForSecretTests()}
			_, err := newTestService(repo).PutSecret(t.Context(), test.namespace, test.spec)

			if !errors.Is(err, test.wantErr) {
				t.Fatalf("PutSecret() error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr != nil && len(repo.secrets) != 0 {
				t.Errorf("a refused request wrote %v", repo.secrets)
			}
			if test.wantErr == nil && len(repo.secrets) != 1 {
				t.Errorf("PutSecret() wrote %d secrets, want 1", len(repo.secrets))
			}
		})
	}
}

// Overwrite decides which write is made, and it has to be said. An existing
// Secret replaced without asking is a running release losing the credential it
// was started with.
func TestPutSecretOverwriteChoosesTheWrite(t *testing.T) {
	tests := []struct {
		name          string
		overwrite     bool
		wantOverwrote bool
	}{
		{name: "by default it creates", overwrite: false, wantOverwrote: false},
		{name: "overwrite updates", overwrite: true, wantOverwrote: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepo{namespaces: namespacesForSecretTests()}
			_, err := newTestService(repo).PutSecret(t.Context(), "apps", SecretSpec{
				Name:      "octo-database",
				Data:      map[string]string{"password": "hunter2"},
				Overwrite: test.overwrite,
			})
			if err != nil {
				t.Fatalf("PutSecret() error = %v", err)
			}
			if repo.secrets[0].overwrote != test.wantOverwrote {
				t.Errorf("overwrote = %v, want %v", repo.secrets[0].overwrote, test.wantOverwrote)
			}
		})
	}
}

// The managed-by label is applied over the caller's, so a Secret cannot claim to
// have been created by something else — and so one this panel wrote is
// answerable later.
func TestPutSecretStampsItsOwnLabel(t *testing.T) {
	repo := &fakeRepo{namespaces: namespacesForSecretTests()}
	ref, err := newTestService(repo).PutSecret(t.Context(), "apps", SecretSpec{
		Name:   "octo-database",
		Data:   map[string]string{"password": "hunter2", "username": "octo"},
		Labels: map[string]string{labelManagedBy: "somebody-else", "team": "lab"},
	})
	if err != nil {
		t.Fatalf("PutSecret() error = %v", err)
	}

	written := repo.secrets[0].spec.Labels
	if written[labelManagedBy] != managedByValue {
		t.Errorf("%s = %q, want %q", labelManagedBy, written[labelManagedBy], managedByValue)
	}
	if written["team"] != "lab" {
		t.Errorf("a caller's own label was dropped: %v", written)
	}

	// The response says what is in the Secret without saying what it holds.
	if !slices.Equal(ref.Keys, []string{"password", "username"}) {
		t.Errorf("Keys = %v, want [password username] sorted", ref.Keys)
	}
}

// A conflict from the cluster stays a conflict. The handler maps it to 409, which
// is what tells the panel to offer replacing rather than to report a failure.
func TestPutSecretPassesAConflictThrough(t *testing.T) {
	repo := &fakeRepo{namespaces: namespacesForSecretTests(), secretErr: ErrAlreadyExists}
	_, err := newTestService(repo).PutSecret(t.Context(), "apps", SecretSpec{
		Name: "octo-database", Data: map[string]string{"password": "hunter2"},
	})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("PutSecret() error = %v, want %v", err, ErrAlreadyExists)
	}
}

// namespacesForSecretTests is what ReadNamespace answers with. The labelled one
// carries protection applied to the live object rather than to configuration,
// which is the case a rule reading only the name would miss.
func namespacesForSecretTests() map[string]Namespace {
	return maps.Collect(func(yield func(string, Namespace) bool) {
		for _, namespace := range []Namespace{
			{Name: "apps"},
			{Name: "admin"},
			{Name: "platform-system"},
			{Name: "labelled", Labels: map[string]string{"home-lab.example/protected": "true"}},
		} {
			if !yield(namespace.Name, namespace) {
				return
			}
		}
	})
}
