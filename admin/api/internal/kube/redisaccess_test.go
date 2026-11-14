package kube

import (
	"errors"
	"testing"
)

// The label is the whole of what these two do, so the tests are about which
// label reaches the cluster, with which value, and when nothing should.
func TestRedisAccess(t *testing.T) {
	granted := redisAccessGranted

	tests := []struct {
		name      string
		namespace string
		grant     bool
		wantPatch *labelPatch
		wantErr   error
	}{
		{
			name:      "granting writes the one label as true",
			namespace: "apps",
			grant:     true,
			wantPatch: &labelPatch{namespace: "apps", key: LabelRedisAccess, value: &granted},
		},
		{
			// nil is "remove the key" in a merge patch, as opposed to setting it to
			// "false", which the NetworkPolicy would also refuse but which leaves a
			// label behind that reads as a decision.
			name:      "revoking removes the label rather than setting it false",
			namespace: "apps",
			grant:     false,
			wantPatch: &labelPatch{namespace: "apps", key: LabelRedisAccess, value: nil},
		},
		{
			name:      "the panel's own namespace is refused, so it cannot cut off its own sign-in",
			namespace: "admin",
			grant:     false,
			wantErr:   ErrProtected,
		},
		{
			name:      "a namespace protected by configuration is refused",
			namespace: "platform-system",
			grant:     true,
			wantErr:   ErrProtected,
		},
		{
			name:      "a namespace that does not exist is not found, and nothing is patched",
			namespace: "nowhere",
			grant:     true,
			wantErr:   ErrNotFound,
		},
		{
			name:      "an empty name is refused before the cluster is asked",
			namespace: "",
			grant:     true,
			wantErr:   ErrInvalidName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{namespaces: map[string]Namespace{
				"apps":            {Name: "apps"},
				"admin":           {Name: "admin"},
				"platform-system": {Name: "platform-system"},
			}}
			service := newTestService(repo)

			var err error
			if tt.grant {
				_, err = service.GrantRedisAccess(t.Context(), tt.namespace)
			} else {
				_, err = service.RevokeRedisAccess(t.Context(), tt.namespace)
			}

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				if len(repo.labelled) != 0 {
					t.Fatalf("a refused request still patched %v", repo.labelled)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(repo.labelled) != 1 {
				t.Fatalf("patched %d times, want exactly once", len(repo.labelled))
			}
			got := repo.labelled[0]
			if got.namespace != tt.wantPatch.namespace || got.key != tt.wantPatch.key {
				t.Errorf("patched %s/%s, want %s/%s", got.namespace, got.key, tt.wantPatch.namespace, tt.wantPatch.key)
			}
			if (got.value == nil) != (tt.wantPatch.value == nil) ||
				(got.value != nil && *got.value != *tt.wantPatch.value) {
				t.Errorf("patched value %v, want %v", got.value, tt.wantPatch.value)
			}
		})
	}
}
