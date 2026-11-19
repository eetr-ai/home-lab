package kube

import (
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// Listing decorates each row with whether the panel will delete or edit it, so
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
			name:   "edit",
			method: "PATCH",
			target: "/api/kubernetes/namespaces/apps/secrets/octo-database",
			body:   `{"replace":{"password":"` + sentinel + `"},"add":{"apikey":"` + sentinel + `"}}`,
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
			// The edit sends values in, so finding one in the response would
			// mean the API echoed back a credential — which is the same leak by a
			// shorter route.
			if strings.Contains(recorder.Body.String(), sentinel) {
				t.Errorf("a response carried a value: %s", recorder.Body)
			}
		})
	}
}

// An edit changes the keys it names and must not disturb the ones it does not.
// The panel cannot see them, so nothing else is in a position to notice.
func TestEditSecretLeavesTheOtherKeysAlone(t *testing.T) {
	repo := &fakeRepo{namespaces: namespacesForSecretTests(), live: liveSecretsForTests()}
	ref, err := newTestService(repo).EditSecret(t.Context(), "apps", "octo-database", SecretEdit{
		Replace: map[string]string{"password": "new-password"},
		Add:     map[string]string{"apikey": "new-key"},
		Remove:  []string{"database"},
	})
	if err != nil {
		t.Fatalf("EditSecret() error = %v", err)
	}

	if len(repo.edited) != 1 {
		t.Fatalf("edited %d times, want 1", len(repo.edited))
	}
	want := map[string]string{"username": "octo", "password": "new-password", "apikey": "new-key"}
	if got := repo.edited[0]; !maps.Equal(got, want) {
		t.Errorf("Secret after the edit = %v, want %v", got, want)
	}

	// The keys the Secret has now — what a chart's existingSecret can point at —
	// not the keys the request named. "database" is gone and must not be claimed.
	if !slices.Equal(ref.Keys, []string{"apikey", "password", "username"}) {
		t.Errorf("Keys = %v, want [apikey password username]", ref.Keys)
	}
}

// Every rule the two write paths share, and the property that matters most: a
// refused request must not have reached the cluster.
func TestSecretWritesRefuseBeforeTheyReachTheCluster(t *testing.T) {
	tests := []struct {
		name       string
		namespace  string
		secretName string
		edit       SecretEdit
		// editOnly marks a case about the edit's payload, which a delete does not
		// carry — so the delete is not expected to refuse it.
		editOnly bool
		wantErr  error
	}{
		{
			name:       "a Secret the panel wrote is edited and deleted",
			namespace:  "apps",
			secretName: "octo-database",
			edit:       SecretEdit{Replace: map[string]string{"password": "new-password"}},
		},
		// G101 fires on the edit maps below, whose keys are called "password"
		// and "token" because that is what a real Secret's keys are called. The
		// values beside them are fixtures.
		//nolint:gosec // G101: Secret key names in a fixture, not credentials.
		{
			// The one that makes the delete grant safe to hold. A release's every
			// revision lives in a Secret, and nothing else has a copy.
			name:       "Helm's release storage is refused",
			namespace:  "apps",
			secretName: "sh.helm.release.v1.octo.v3",
			edit:       SecretEdit{Replace: map[string]string{"release": "anything"}},
			wantErr:    ErrReserved,
		},
		{
			name:       "a ServiceAccount token is refused",
			namespace:  "apps",
			secretName: "octo-token",
			edit:       SecretEdit{Replace: map[string]string{"token": "anything"}},
			wantErr:    ErrReserved,
		},
		{
			// Kubernetes refuses every write to one. Saying so here is a 403 that
			// names the Secret rather than a 500 wrapping a field-path message.
			name:       "an immutable Secret is refused",
			namespace:  "apps",
			secretName: "octo-pinned",
			edit:       SecretEdit{Replace: map[string]string{"password": "new-password"}},
			wantErr:    ErrReserved,
		},
		{
			// Replace means "this key, new value". A typo in a rotation must be
			// refused rather than land as a second key beside the credential the
			// operator believes they replaced.
			name:       "replacing a key the Secret does not have is refused",
			namespace:  "apps",
			secretName: "octo-database",
			edit:       SecretEdit{Replace: map[string]string{"pasword": "new-value"}},
			editOnly:   true,
			wantErr:    ErrInvalidName,
		},
		{
			// And the other way round: adding is not a quiet overwrite.
			name:       "adding a key the Secret already has is refused",
			namespace:  "apps",
			secretName: "octo-database",
			edit:       SecretEdit{Add: map[string]string{"password": "new-value"}},
			editOnly:   true,
			wantErr:    ErrAlreadyExists,
		},
		{
			name:       "removing a key the Secret does not have is refused",
			namespace:  "apps",
			secretName: "octo-database",
			edit:       SecretEdit{Remove: []string{"apikey"}},
			editOnly:   true,
			wantErr:    ErrInvalidName,
		},
		{
			// That is a delete, and the delete has its own confirmation.
			name:       "removing every key is refused",
			namespace:  "apps",
			secretName: "octo-database",
			edit:       SecretEdit{Remove: []string{"database", "password", "username"}},
			editOnly:   true,
			wantErr:    ErrInvalidName,
		},
		{
			name:       "removing every key while adding one is an edit",
			namespace:  "apps",
			secretName: "octo-database",
			edit: SecretEdit{
				Add:    map[string]string{"apikey": "new-value"},
				Remove: []string{"database", "password", "username"},
			},
			editOnly: true,
		},
		{
			// No order of applying it is obviously what was meant.
			name:       "a key named in two lists is refused",
			namespace:  "apps",
			secretName: "octo-database",
			edit: SecretEdit{
				Replace: map[string]string{"password": "new-value"},
				Remove:  []string{"password"},
			},
			editOnly: true,
			wantErr:  ErrInvalidName,
		},
		{
			name:       "a key named twice in remove is refused",
			namespace:  "apps",
			secretName: "octo-database",
			edit:       SecretEdit{Remove: []string{"password", "password"}},
			editOnly:   true,
			wantErr:    ErrInvalidName,
		},
		{
			name:       "an added key Kubernetes would not accept is refused",
			namespace:  "apps",
			secretName: "octo-database",
			edit:       SecretEdit{Add: map[string]string{"api key": "new-value"}},
			editOnly:   true,
			wantErr:    ErrInvalidName,
		},
		{
			// A key becomes a filename when the Secret is mounted, so Kubernetes
			// refuses "." and anything starting with "..". Rotation never reached
			// this because the key had to exist; adding can.
			name:       "adding the key . is refused",
			namespace:  "apps",
			secretName: "octo-database",
			edit:       SecretEdit{Add: map[string]string{".": "new-value"}},
			editOnly:   true,
			wantErr:    ErrInvalidName,
		},
		{
			name:       "adding a key starting with .. is refused",
			namespace:  "apps",
			secretName: "octo-database",
			edit:       SecretEdit{Add: map[string]string{"..name": "new-value"}},
			editOnly:   true,
			wantErr:    ErrInvalidName,
		},
		{
			name:       "adding a key starting with a single dot is an edit",
			namespace:  "apps",
			secretName: "octo-database",
			edit:       SecretEdit{Add: map[string]string{".env": "new-value"}},
			editOnly:   true,
		},
		{
			name:       "an edit that changes nothing is refused",
			namespace:  "apps",
			secretName: "octo-database",
			edit:       SecretEdit{},
			editOnly:   true,
			wantErr:    ErrInvalidName,
		},
		{
			name:       "an empty value is refused",
			namespace:  "apps",
			secretName: "octo-database",
			edit:       SecretEdit{Replace: map[string]string{"password": ""}},
			editOnly:   true,
			wantErr:    ErrInvalidName,
		},
		{
			name:       "an empty added value is refused",
			namespace:  "apps",
			secretName: "octo-database",
			edit:       SecretEdit{Add: map[string]string{"apikey": ""}},
			editOnly:   true,
			wantErr:    ErrInvalidName,
		},
		{
			name:       "a protected namespace is refused",
			namespace:  "admin",
			secretName: "octo-database",
			edit:       SecretEdit{Replace: map[string]string{"password": "new-password"}},
			wantErr:    ErrProtected,
		},
		{
			name:       "a namespace the panel does not manage is refused",
			namespace:  "unmanaged",
			secretName: "octo-database",
			edit:       SecretEdit{Replace: map[string]string{"password": "new-password"}},
			wantErr:    ErrNotManaged,
		},
		{
			name:       "a malformed namespace is refused",
			namespace:  "Apps",
			secretName: "octo-database",
			edit:       SecretEdit{Replace: map[string]string{"password": "new-password"}},
			wantErr:    ErrInvalidName,
		},
		{
			name:       "a malformed Secret name is refused",
			namespace:  "apps",
			secretName: "Octo Database",
			edit:       SecretEdit{Replace: map[string]string{"password": "new-password"}},
			wantErr:    ErrInvalidName,
		},
		{
			name:       "a Secret that is not there is a not-found",
			namespace:  "apps",
			secretName: "missing",
			edit:       SecretEdit{Replace: map[string]string{"password": "new-password"}},
			wantErr:    ErrNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("edit", func(t *testing.T) {
				repo := &fakeRepo{namespaces: namespacesForSecretTests(), live: liveSecretsForTests()}
				_, err := newTestService(repo).EditSecret(t.Context(), test.namespace, test.secretName,
					test.edit)

				if !errors.Is(err, test.wantErr) {
					t.Fatalf("EditSecret() error = %v, want %v", err, test.wantErr)
				}
				if test.wantErr != nil && len(repo.edited) != 0 {
					t.Errorf("a refused edit wrote %v", repo.edited)
				}
				if test.wantErr == nil && len(repo.edited) != 1 {
					t.Errorf("EditSecret() wrote %d times, want 1", len(repo.edited))
				}
			})

			t.Run("delete", func(t *testing.T) {
				if test.editOnly {
					t.Skip("a delete carries no data")
				}

				repo := &fakeRepo{namespaces: namespacesForSecretTests(), live: liveSecretsForTests()}
				err := newTestService(repo).DeleteSecret(t.Context(), test.namespace, test.secretName)

				wantErr := test.wantErr
				// An immutable Secret can be deleted; immutability is a rule about
				// changing one, not about removing it. Editing is where it bites.
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
				// The identity, not just the name. It is what the repository turns
				// into a precondition, and without it a delete would be free to
				// land on whatever holds that name by the time it arrives.
				if wantErr == nil && len(repo.removed) == 1 {
					if got := repo.removed[0]; got.uid == "" || got.resourceVersion == "" {
						t.Errorf("DeleteSecret() passed uid %q version %q, want the live ones",
							got.uid, got.resourceVersion)
					}
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

				uid:             "6f0b2a1e-0000-4000-8000-000000000001",
				resourceVersion: "4711",
			},
			data: map[string]string{"username": "octo", "password": "hunter2", "database": "octo"},
		},
		"sh.helm.release.v1.octo.v3": {
			summary: SecretSummary{
				Name: "sh.helm.release.v1.octo.v3",
				Type: "helm.sh/release.v1",
				Keys: []string{"release"},

				uid:             "6f0b2a1e-0000-4000-8000-000000000002",
				resourceVersion: "4712",
			},
			data: map[string]string{"release": "a base64 gzip of the whole release"},
		},
		"octo-token": {
			summary: SecretSummary{
				Name: "octo-token",
				Type: "kubernetes.io/service-account-token",
				Keys: []string{"ca.crt", "namespace", "token"},

				uid:             "6f0b2a1e-0000-4000-8000-000000000003",
				resourceVersion: "4713",
			},
			data: map[string]string{"token": "an issued token"},
		},
		"octo-pinned": {
			summary: SecretSummary{
				Name:      "octo-pinned",
				Type:      "Opaque",
				Keys:      []string{"password"},
				Immutable: true,

				uid:             "6f0b2a1e-0000-4000-8000-000000000004",
				resourceVersion: "4714",
			},
			data: map[string]string{"password": "hunter2"},
		},
	}
}

// The service's verdict is about a Secret it read; the write lands on whatever
// holds that name a moment later. These two guard that gap.
//
// editable is the repository's re-check, made inside the retry loop against the
// object the Update is about to replace. The cases below are the ways that object
// could have become one this edit must not be applied to between the read and the
// write.
func TestEditableJudgesTheObjectBeingWritten(t *testing.T) {
	pinned := true
	tests := []struct {
		name    string
		live    *corev1.Secret
		edit    SecretEdit
		wantErr error
	}{
		{
			name: "the Secret that was checked",
			live: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "octo-database"},
				Type:       corev1.SecretTypeOpaque,
				Data:       map[string][]byte{"password": []byte("hunter2")},
			},
			edit: SecretEdit{Replace: map[string]string{"password": "new-password"}},
		},
		{
			name: "replaced by Helm's release storage",
			live: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "octo-database"},
				Type:       corev1.SecretType(helmReleaseSecretType),
				Data:       map[string][]byte{"password": []byte("hunter2")},
			},
			edit:    SecretEdit{Replace: map[string]string{"password": "new-password"}},
			wantErr: ErrReserved,
		},
		{
			name: "made immutable since the read",
			live: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "octo-database"},
				Type:       corev1.SecretTypeOpaque,
				Immutable:  &pinned,
				Data:       map[string][]byte{"password": []byte("hunter2")},
			},
			edit:    SecretEdit{Replace: map[string]string{"password": "new-password"}},
			wantErr: ErrReserved,
		},
		{
			name: "no longer has the key being replaced",
			live: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "octo-database"},
				Type:       corev1.SecretTypeOpaque,
				Data:       map[string][]byte{"username": []byte("octo")},
			},
			edit:    SecretEdit{Replace: map[string]string{"password": "new-password"}},
			wantErr: ErrInvalidName,
		},
		{
			name: "has gained the key being added",
			live: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "octo-database"},
				Type:       corev1.SecretTypeOpaque,
				Data:       map[string][]byte{"password": []byte("hunter2"), "apikey": []byte("theirs")},
			},
			edit:    SecretEdit{Add: map[string]string{"apikey": "mine"}},
			wantErr: ErrAlreadyExists,
		},
		{
			// Somebody else removed the other keys; this removal would now
			// empty the Secret, which is a delete nobody confirmed.
			name: "would be left with no keys",
			live: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "octo-database"},
				Type:       corev1.SecretTypeOpaque,
				Data:       map[string][]byte{"password": []byte("hunter2")},
			},
			edit:    SecretEdit{Remove: []string{"password"}},
			wantErr: ErrInvalidName,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := editable(test.live, test.edit); !errors.Is(err, test.wantErr) {
				t.Errorf("editable() error = %v, want %v", err, test.wantErr)
			}
		})
	}
}

// And the identity a delete is bound to has to survive the projection, because
// the projection is the only thing the service ever sees. Empty preconditions
// would be refused by the API server rather than ignored, so this failing would
// be loud — but it would be loud in a cluster rather than here.
func TestSummarizeSecretCarriesTheIdentityAWriteIsBoundTo(t *testing.T) {
	summary := summarizeSecret(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "octo-database",
			UID:             "6f0b2a1e-0000-4000-8000-000000000001",
			ResourceVersion: "4711",
		},
		Type: corev1.SecretTypeOpaque,
	})

	if summary.uid != "6f0b2a1e-0000-4000-8000-000000000001" || summary.resourceVersion != "4711" {
		t.Errorf("summarizeSecret() uid = %q version = %q, want the live ones",
			summary.uid, summary.resourceVersion)
	}
}

// The merge, on the real object. Every key not named comes through byte for byte,
// and StringData cannot survive to bring a removed key back or override a
// replaced one.
func TestApplySecretEditChangesOnlyTheNamedKeys(t *testing.T) {
	live := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "octo-database"},
		Type:       corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"username": []byte("octo"),
			"password": []byte("hunter2"),
			"database": []byte("octo"),
		},
		StringData: map[string]string{"database": "stale", "pending": "not yet folded"},
	}

	applySecretEdit(live, SecretEdit{
		Replace: map[string]string{"password": "new-password"},
		Add:     map[string]string{"apikey": "new-key"},
		Remove:  []string{"database"},
	})

	got := map[string]string{}
	for key, value := range live.Data {
		got[key] = string(value)
	}
	want := map[string]string{
		"username": "octo",
		"password": "new-password",
		"apikey":   "new-key",
		"pending":  "not yet folded",
	}
	if !maps.Equal(got, want) {
		t.Errorf("Data = %v, want %v", got, want)
	}
	if live.StringData != nil {
		t.Errorf("StringData = %v, want it cleared", live.StringData)
	}
}

// The response describes the object that was written. A key another writer added
// after the service's read survives the merge, so it has to be in the answer.
func TestEditSecretReportsKeysFromTheWrite(t *testing.T) {
	live := liveSecretsForTests()
	secret := live["octo-database"]
	secret.data["region"] = "added by somebody else after the read"
	live["octo-database"] = secret

	repo := &fakeRepo{namespaces: namespacesForSecretTests(), live: live}
	ref, err := newTestService(repo).EditSecret(t.Context(), "apps", "octo-database",
		SecretEdit{Replace: map[string]string{"password": "new-password"}})
	if err != nil {
		t.Fatalf("EditSecret() error = %v", err)
	}
	if !slices.Contains(ref.Keys, "region") {
		t.Errorf("Keys = %v, want the key the write kept", ref.Keys)
	}
}

// encoding/json keeps the last of two members with one name, so a request with
// two values for one key would otherwise succeed with whichever came second.
func TestEditSecretRefusesAKeyRepeatedInsideOneObject(t *testing.T) {
	bodies := map[string]string{
		"replace": `{"replace":{"password":"a","password":"b"}}`,
		"add":     `{"add":{"apikey":"a","apikey":"b"}}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			repo := &fakeRepo{namespaces: namespacesForSecretTests(), live: liveSecretsForTests()}
			mux := http.NewServeMux()
			NewHandler(newTestService(repo)).Register(mux)

			req := httptest.NewRequestWithContext(t.Context(), "PATCH",
				"/api/kubernetes/namespaces/apps/secrets/octo-database", strings.NewReader(body))
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, req)

			if recorder.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400: %s", recorder.Code, recorder.Body)
			}
			if len(repo.edited) != 0 {
				t.Errorf("a refused edit wrote %v", repo.edited)
			}
		})
	}
}

// The handler's own decoding must still accept what it used to: absent maps,
// null maps, and an ordinary edit.
func TestDecodeUniqueStringMap(t *testing.T) {
	tests := []struct {
		raw     string
		want    map[string]string
		wantErr bool
	}{
		{raw: ``, want: nil},
		{raw: `null`, want: nil},
		{raw: `{}`, want: map[string]string{}},
		{raw: `{"a":"1","b":"2"}`, want: map[string]string{"a": "1", "b": "2"}},
		{raw: `{"a":"1","a":"2"}`, wantErr: true},
		{raw: `{"a":1}`, wantErr: true},
		{raw: `["a"]`, wantErr: true},
	}
	for _, test := range tests {
		got, err := decodeUniqueStringMap([]byte(test.raw))
		if (err != nil) != test.wantErr {
			t.Errorf("decodeUniqueStringMap(%s) error = %v, wantErr %v", test.raw, err, test.wantErr)
			continue
		}
		if !test.wantErr && !maps.Equal(got, test.want) {
			t.Errorf("decodeUniqueStringMap(%s) = %v, want %v", test.raw, got, test.want)
		}
	}
}
