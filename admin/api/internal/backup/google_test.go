package backup

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// pointGoogleAt swaps Google's endpoints for a local server for one test.
func pointGoogleAt(t *testing.T, server *httptest.Server) {
	t.Helper()
	auth, token, revoke := googleAuthURL, googleTokenURL, googleRevokeURL
	googleAuthURL, googleTokenURL, googleRevokeURL = server.URL+"/auth", server.URL+"/token", server.URL+"/revoke"
	t.Cleanup(func() { googleAuthURL, googleTokenURL, googleRevokeURL = auth, token, revoke })
}

func idToken(payload string) string {
	return "header." + base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".signature"
}

// The exchange is where PKCE either holds or does not, so the test reads the form
// Google would receive rather than trusting the library to have sent it.
func TestExchangeSendsTheVerifierAndReadsTheAccount(t *testing.T) {
	var form map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = map[string]string{}
		for key := range r.PostForm {
			form[key] = r.PostForm.Get(key)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"a","refresh_token":"r","expires_in":3600,` +
			`"id_token":"` + idToken(`{"email":"op@example.invalid"}`) + `"}`))
	}))
	defer server.Close()
	pointGoogleAt(t, server)

	grant, err := NewGoogleClient("client", "secret").
		Exchange(context.Background(), "the-code", "the-verifier", "https://panel.example.invalid/cb")
	if err != nil {
		t.Fatal(err)
	}

	if grant.RefreshToken != "r" || grant.Account != "op@example.invalid" {
		t.Errorf("unexpected grant %+v", grant)
	}
	want := map[string]string{
		"code": "the-code", "code_verifier": "the-verifier",
		"redirect_uri": "https://panel.example.invalid/cb", "client_id": "client",
	}
	for key, value := range want {
		if form[key] != value {
			t.Errorf("%s should be %q, was %q", key, value, form[key])
		}
	}
}

func TestExchangeWithoutOfflineAccessIsARefusalThatSaysWhatToDo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"a","expires_in":3600}`))
	}))
	defer server.Close()
	pointGoogleAt(t, server)

	_, err := NewGoogleClient("client", "secret").
		Exchange(context.Background(), "code", "verifier", "https://panel.example.invalid/cb")
	if !errors.Is(err, ErrGoogleRefused) || !strings.Contains(err.Error(), "connect again") {
		t.Errorf("expected a refusal telling the operator to reconnect, got %v", err)
	}
}

func TestGoogleSayingNoIsARefusalAndKeepsItsReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
	}))
	defer server.Close()
	pointGoogleAt(t, server)

	_, err := NewGoogleClient("client", "secret").AccessToken(context.Background(), "refresh")
	if !errors.Is(err, ErrGoogleRefused) || !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("expected a refusal naming invalid_grant, got %v", err)
	}
}

func TestRevokingAGrantGoogleAlreadyForgotIsSuccess(t *testing.T) {
	tests := []struct {
		name   string
		status int
		ok     bool
	}{
		{"revoked now", http.StatusOK, true},
		{"already gone", http.StatusBadRequest, true},
		{"google failing", http.StatusInternalServerError, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
			}))
			defer server.Close()
			pointGoogleAt(t, server)

			err := NewGoogleClient("client", "secret").Revoke(context.Background(), "refresh")
			if (err == nil) != test.ok {
				t.Errorf("status %d: got %v", test.status, err)
			}
		})
	}
}

func TestAnUnreadableIDTokenLeavesTheAccountBlank(t *testing.T) {
	tests := []string{"", "not-a-jwt", "a.!!!.c", idToken("not json")}
	for _, raw := range tests {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"a","refresh_token":"r","id_token":"` + raw + `"}`))
		}))
		pointGoogleAt(t, server)

		grant, err := NewGoogleClient("c", "s").Exchange(context.Background(), "code", "v", "https://x.invalid/cb")
		server.Close()
		if err != nil || grant.Account != "" {
			t.Errorf("id_token %q: expected a grant with no account, got %+v, %v", raw, grant, err)
		}
	}
}
