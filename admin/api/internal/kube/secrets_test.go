package kube

import (
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
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
		{
			// The grant that permits this write is the one that makes a namespace
			// Helm-managed. Refusing here says so; the API server's own 403 would
			// read as a broken role binding.
			name:      "a namespace the panel does not manage is refused",
			namespace: "unmanaged",
			spec:      SecretSpec{Name: "octo-database", Data: map[string]string{"password": "hunter2"}},
			wantErr:   ErrNotManaged,
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
			{Name: "apps", Labels: map[string]string{"home-lab.example/helm-managed": "true"}},
			{Name: "unmanaged"},
			// Managed and protected at once, which is the panel's own namespace
			// exactly: it deploys itself, so the Helm grant applies here, and
			// protection is what must still refuse a write.
			{Name: "admin", Labels: map[string]string{"home-lab.example/helm-managed": "true"}},
			{Name: "platform-system"},
			{Name: "labelled", Labels: map[string]string{"home-lab.example/protected": "true"}},
		} {
			if !yield(namespace.Name, namespace) {
				return
			}
		}
	})
}

// The deny-list is the containment for the delete grant, so its contents are
// worth pinning rather than trusting to a switch statement staying right.
//
// The negative cases matter as much as the positive ones: a deny-list that grew
// to cover every way of breaking a namespace would be a deny-list on managing
// one, and TLS material and registry credentials are things an operator replaces
// on purpose.
func TestReservedSecretNamesWhatThePanelWillNotTouch(t *testing.T) {
	//nolint:gosec // G101: Kubernetes Secret *type* names, not credentials.
	tests := []struct {
		secretType string
		want       bool
	}{
		{secretType: "helm.sh/release.v1", want: true},
		{secretType: "kubernetes.io/service-account-token", want: true},
		{secretType: "Opaque", want: false},
		{secretType: "kubernetes.io/tls", want: false},
		{secretType: "kubernetes.io/dockerconfigjson", want: false},
		{secretType: "kubernetes.io/basic-auth", want: false},
		{secretType: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.secretType, func(t *testing.T) {
			got, reason := reservedSecret(test.secretType)
			if got != test.want {
				t.Fatalf("reservedSecret(%q) = %v, want %v", test.secretType, got, test.want)
			}
			// A refusal that does not say why leaves the panel drawing a disabled
			// button with nothing to put beside it.
			if got && reason == "" {
				t.Error("a reserved type gave no reason")
			}
			if !got && reason != "" {
				t.Errorf("a permitted type gave a reason: %q", reason)
			}
		})
	}
}

// Listing decorates each row with whether the panel will delete or rotate it, so
// that the panel and the write paths cannot disagree about the same Secret.
//
// The protected namespace is the interesting one: it lists, because the panel's
// own namespace holding the credentials it runs on is exactly what an operator
// wants to see, and every row comes back refused.
func TestListSecretsSaysWhatMayBeTouched(t *testing.T) {
	tests := []struct {
		name       string
		namespace  string
		wantErr    error
		wantOpaque bool
		wantHelm   bool
	}{
		{
			name:       "a managed namespace lists, and Helm's own is refused",
			namespace:  "apps",
			wantOpaque: true,
			wantHelm:   false,
		},
		{
			// Protection is about writing. Hiding the rows would take away the
			// reading this section exists for.
			name:       "a protected namespace lists with nothing removable",
			namespace:  "admin",
			wantOpaque: false,
			wantHelm:   false,
		},
		{
			name:      "a namespace the panel does not manage is refused",
			namespace: "unmanaged",
			wantErr:   ErrNotManaged,
		},
		{
			name:      "a malformed namespace is refused before the cluster is asked",
			namespace: "Apps",
			wantErr:   ErrInvalidName,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeRepo{namespaces: namespacesForSecretTests(), live: liveSecretsForTests()}
			secrets, err := newTestService(repo).ListSecrets(t.Context(), test.namespace)

			if !errors.Is(err, test.wantErr) {
				t.Fatalf("ListSecrets() error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr != nil {
				return
			}

			byName := map[string]SecretSummary{}
			for _, secret := range secrets {
				byName[secret.Name] = secret
			}
			if got := byName["octo-database"].Removable; got != test.wantOpaque {
				t.Errorf("octo-database Removable = %v, want %v", got, test.wantOpaque)
			}
			if got := byName["sh.helm.release.v1.octo.v3"].Removable; got != test.wantHelm {
				t.Errorf("the Helm release Secret Removable = %v, want %v", got, test.wantHelm)
			}
			for _, secret := range secrets {
				if !secret.Removable && secret.Reason == "" {
					t.Errorf("%s is not removable and says nothing about why", secret.Name)
				}
			}
		})
	}
}

// The invariant the whole feature rests on: no route in this slice returns a
// value, and there is no endpoint that reveals one.
//
// Asserted over the response body rather than over the struct, because the way
// this breaks is somebody adding a field to SecretSummary — which a test reading
// named fields would go on passing through.
func TestNoSecretResponseCarriesAValue(t *testing.T) {
	const sentinel = "hunter2-do-not-serialise"

	repo := &fakeRepo{namespaces: namespacesForSecretTests(), live: liveSecretsForTests()}
	for name, secret := range repo.live {
		secret.data = map[string]string{"password": sentinel}
		repo.live[name] = secret
	}
	service := newTestService(repo)

	mux := http.NewServeMux()
	NewHandler(service).Register(mux)

	requests := []struct {
		name    string
		method  string
		target  string
		body    string
		wantMax int
	}{
		{name: "list", method: "GET", target: "/api/kubernetes/namespaces/apps/secrets", wantMax: 200},
		{
			name:   "rotate",
			method: "POST",
			target: "/api/kubernetes/namespaces/apps/secrets/octo-database/rotate",
			body:   `{"data":{"password":"` + sentinel + `"}}`,
		},
	}

	for _, request := range requests {
		t.Run(request.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(
				t.Context(), request.method, request.target, strings.NewReader(request.body))
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)

			if recorder.Code >= 300 {
				t.Fatalf("%s %s = %d: %s", request.method, request.target, recorder.Code, recorder.Body)
			}
			// The rotation sends the value in, so finding it in the response would
			// mean the API echoed back a credential — which is the same leak by a
			// shorter route.
			if strings.Contains(recorder.Body.String(), sentinel) {
				t.Errorf("a response carried a value: %s", recorder.Body)
			}
		})
	}
}

// Rotation replaces a value and must not disturb the keys it was not asked
// about. The panel cannot see them, so nothing else is in a position to notice.
func TestRotateSecretLeavesTheOtherKeysAlone(t *testing.T) {
	repo := &fakeRepo{namespaces: namespacesForSecretTests(), live: liveSecretsForTests()}
	ref, err := newTestService(repo).RotateSecret(t.Context(), "apps", "octo-database",
		SecretRotation{Data: map[string]string{"password": "new-password"}})
	if err != nil {
		t.Fatalf("RotateSecret() error = %v", err)
	}

	if len(repo.rotated) != 1 {
		t.Fatalf("rotated %d times, want 1", len(repo.rotated))
	}
	got := repo.rotated[0]
	if got["password"] != "new-password" {
		t.Errorf("password = %q, want the new one", got["password"])
	}
	if got["username"] != "octo" {
		t.Errorf("username = %q, want it untouched", got["username"])
	}
	if got["database"] != "octo" {
		t.Errorf("database = %q, want it untouched", got["database"])
	}

	// The response names the keys that changed, not the keys the Secret has:
	// saying "username" here would claim a rotation that did not happen.
	if !slices.Equal(ref.Keys, []string{"password"}) {
		t.Errorf("Keys = %v, want [password]", ref.Keys)
	}
}

// Every rule the two write paths share, and the property that matters most: a
// refused request must not have reached the cluster.
func TestSecretWritesRefuseBeforeTheyReachTheCluster(t *testing.T) {
	tests := []struct {
		name       string
		namespace  string
		secretName string
		rotation   map[string]string
		wantErr    error
	}{
		{
			name:       "a Secret the panel wrote is rotated and deleted",
			namespace:  "apps",
			secretName: "octo-database",
			rotation:   map[string]string{"password": "new-password"},
		},
		// G101 fires on the rotation maps below, whose keys are called "password"
		// and "token" because that is what a real Secret's keys are called. The
		// values beside them are fixtures.
		//nolint:gosec // G101: Secret key names in a fixture, not credentials.
		{
			// The one that makes the delete grant safe to hold. A release's every
			// revision lives in a Secret, and nothing else has a copy.
			name:       "Helm's release storage is refused",
			namespace:  "apps",
			secretName: "sh.helm.release.v1.octo.v3",
			rotation:   map[string]string{"release": "anything"},
			wantErr:    ErrReserved,
		},
		{
			name:       "a ServiceAccount token is refused",
			namespace:  "apps",
			secretName: "octo-token",
			rotation:   map[string]string{"token": "anything"},
			wantErr:    ErrReserved,
		},
		{
			// Kubernetes refuses every write to one. Saying so here is a 403 that
			// names the Secret rather than a 500 wrapping a field-path message.
			name:       "an immutable Secret is refused",
			namespace:  "apps",
			secretName: "octo-pinned",
			rotation:   map[string]string{"password": "new-password"},
			wantErr:    ErrReserved,
		},
		{
			// Rotation replaces; adding a key is PutSecret's job, where replacing
			// what is already there has to be said out loud. Without this rule
			// rotate is a second create with weaker guards.
			name:       "a key the Secret does not have is refused",
			namespace:  "apps",
			secretName: "octo-database",
			rotation:   map[string]string{"apikey": "new-value"},
			wantErr:    ErrInvalidName,
		},
		{
			name:       "an empty value is refused",
			namespace:  "apps",
			secretName: "octo-database",
			rotation:   map[string]string{"password": ""},
			wantErr:    ErrInvalidName,
		},
		{
			name:       "a protected namespace is refused",
			namespace:  "admin",
			secretName: "octo-database",
			rotation:   map[string]string{"password": "new-password"},
			wantErr:    ErrProtected,
		},
		{
			name:       "a namespace the panel does not manage is refused",
			namespace:  "unmanaged",
			secretName: "octo-database",
			rotation:   map[string]string{"password": "new-password"},
			wantErr:    ErrNotManaged,
		},
		{
			name:       "a malformed namespace is refused",
			namespace:  "Apps",
			secretName: "octo-database",
			rotation:   map[string]string{"password": "new-password"},
			wantErr:    ErrInvalidName,
		},
		{
			name:       "a malformed Secret name is refused",
			namespace:  "apps",
			secretName: "Octo Database",
			rotation:   map[string]string{"password": "new-password"},
			wantErr:    ErrInvalidName,
		},
		{
			name:       "a Secret that is not there is a not-found",
			namespace:  "apps",
			secretName: "missing",
			rotation:   map[string]string{"password": "new-password"},
			wantErr:    ErrNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("rotate", func(t *testing.T) {
				repo := &fakeRepo{namespaces: namespacesForSecretTests(), live: liveSecretsForTests()}
				_, err := newTestService(repo).RotateSecret(t.Context(), test.namespace, test.secretName,
					SecretRotation{Data: test.rotation})

				if !errors.Is(err, test.wantErr) {
					t.Fatalf("RotateSecret() error = %v, want %v", err, test.wantErr)
				}
				if test.wantErr != nil && len(repo.rotated) != 0 {
					t.Errorf("a refused rotation wrote %v", repo.rotated)
				}
			})

			t.Run("delete", func(t *testing.T) {
				// The empty-value and unknown-key cases are rotation's alone —
				// a delete carries no data — so they are not expected to refuse
				// here and the row is skipped rather than asserted wrongly.
				if test.name == "an empty value is refused" ||
					test.name == "a key the Secret does not have is refused" {
					t.Skip("a delete carries no data")
				}

				repo := &fakeRepo{namespaces: namespacesForSecretTests(), live: liveSecretsForTests()}
				err := newTestService(repo).DeleteSecret(t.Context(), test.namespace, test.secretName)

				wantErr := test.wantErr
				// An immutable Secret can be deleted; immutability is a rule about
				// changing one, not about removing it. Rotation is where it bites.
				if test.name == "an immutable Secret is refused" {
					wantErr = nil
				}
				if !errors.Is(err, wantErr) {
					t.Fatalf("DeleteSecret() error = %v, want %v", err, wantErr)
				}
				if wantErr != nil && len(repo.removed) != 0 {
					t.Errorf("a refused delete removed %v", repo.removed)
				}
				if wantErr == nil && len(repo.removed) != 1 {
					t.Errorf("DeleteSecret() removed %d, want 1", len(repo.removed))
				}
			})
		})
	}
}

// liveSecretsForTests is what the cluster holds: one the panel wrote, Helm's own
// release storage, a ServiceAccount token, and an immutable one — the four cases
// the write paths have to tell apart.
func liveSecretsForTests() map[string]liveSecret {
	return map[string]liveSecret{
		"octo-database": {
			summary: SecretSummary{
				Name:         "octo-database",
				Type:         "Opaque",
				Keys:         []string{"database", "password", "username"},
				PanelManaged: true,
			},
			data: map[string]string{"username": "octo", "password": "hunter2", "database": "octo"},
		},
		"sh.helm.release.v1.octo.v3": {
			summary: SecretSummary{
				Name: "sh.helm.release.v1.octo.v3",
				Type: "helm.sh/release.v1",
				Keys: []string{"release"},
			},
			data: map[string]string{"release": "a base64 gzip of the whole release"},
		},
		"octo-token": {
			summary: SecretSummary{
				Name: "octo-token",
				Type: "kubernetes.io/service-account-token",
				Keys: []string{"ca.crt", "namespace", "token"},
			},
			data: map[string]string{"token": "an issued token"},
		},
		"octo-pinned": {
			summary: SecretSummary{
				Name:      "octo-pinned",
				Type:      "Opaque",
				Keys:      []string{"password"},
				Immutable: true,
			},
			data: map[string]string{"password": "hunter2"},
		},
	}
}
