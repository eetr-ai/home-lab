package backup

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// fakeGoogle stands in for Google's OAuth endpoints, recording what it was asked
// so a test can check that a refusal happened before Google heard about it.
type fakeGoogle struct {
	grant     Grant
	err       error
	revokeErr error

	exchanged []string
	revoked   []string
}

func (f *fakeGoogle) Exchange(_ context.Context, code, _, _ string) (Grant, error) {
	f.exchanged = append(f.exchanged, code)
	return f.grant, f.err
}

func (f *fakeGoogle) AccessToken(context.Context, string) (AccessToken, error) {
	return AccessToken{AccessToken: "access"}, f.err
}

func (f *fakeGoogle) Revoke(_ context.Context, refreshToken string) error {
	f.revoked = append(f.revoked, refreshToken)
	return f.revokeErr
}

// fakeGrants keeps the grant in memory the way the Secret would.
type fakeGrants struct {
	grant   *Grant
	err     error
	deleted bool
}

func (f *fakeGrants) ReadGrant(context.Context) (Grant, error) {
	if f.err != nil {
		return Grant{}, f.err
	}
	if f.grant == nil {
		return Grant{}, ErrNotConnected
	}
	return *f.grant, nil
}

func (f *fakeGrants) WriteGrant(_ context.Context, grant Grant) error {
	f.grant = &grant
	return f.err
}

func (f *fakeGrants) DeleteGrant(context.Context) error {
	f.grant = nil
	f.deleted = true
	return f.err
}

var testGoogleConfig = GoogleConfig{
	ClientID: "client", ClientSecret: "secret", PickerAPIKey: "key", PickerAppID: "123",
}

func newTestService(google Google, grants GrantStore) *Service {
	return NewService(google, grants, testGoogleConfig, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// A lab with no OAuth client is a supported state, and the page needs to be able
// to say so rather than fail: the status answers, and only the actions refuse.
func TestAnUnconfiguredLabReportsItselfAndRefusesActions(t *testing.T) {
	service := NewService(nil, &fakeGrants{}, GoogleConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	status, err := service.GoogleStatus(context.Background())
	if err != nil || status.Configured {
		t.Fatalf("status should report an unconfigured lab without failing, got %+v, %v", status, err)
	}

	if _, err := service.Connect(context.Background(), validConnect()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("connect should answer not configured, got %v", err)
	}
	if err := service.Disconnect(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("disconnect should answer not configured, got %v", err)
	}
	if _, err := service.AccessToken(context.Background()); !errors.Is(err, ErrNotConfigured) {
		t.Errorf("access token should answer not configured, got %v", err)
	}
}

func TestStatusReportsTheConnectedAccountButNeverTheGrant(t *testing.T) {
	grants := &fakeGrants{grant: &Grant{RefreshToken: "refresh-secret", Account: "op@example.invalid"}}
	status, err := newTestService(&fakeGoogle{}, grants).GoogleStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Configured || !status.Connected || status.Account != "op@example.invalid" {
		t.Errorf("status should show the connected account, got %+v", status)
	}
	if strings.Contains(strings.ToLower(status.ClientID+status.Account+status.PickerAPIKey), "refresh-secret") {
		t.Error("the refresh token must never leave the API")
	}
}

func TestConnectRefusesAMalformedRequestBeforeAskingGoogle(t *testing.T) {
	tests := []struct {
		name    string
		request func(*ConnectRequest)
	}{
		{"no code", func(r *ConnectRequest) { r.Code = " " }},
		{"short verifier", func(r *ConnectRequest) { r.CodeVerifier = "short" }},
		{"long verifier", func(r *ConnectRequest) { r.CodeVerifier = strings.Repeat("v", 129) }},
		{"relative redirect", func(r *ConnectRequest) { r.RedirectURI = "/api/backups/google/callback" }},
		{"odd scheme", func(r *ConnectRequest) { r.RedirectURI = "javascript://x/callback" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			google := &fakeGoogle{}
			request := validConnect()
			test.request(&request)

			_, err := newTestService(google, &fakeGrants{}).Connect(context.Background(), request)
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("expected an invalid request, got %v", err)
			}
			if len(google.exchanged) != 0 {
				t.Error("Google should not have been asked anything")
			}
		})
	}
}

// Connecting a second account replaces the first, and the first one's grant would
// otherwise stay alive at Google with nothing here that remembers it.
func TestConnectReplacesAndRevokesThePreviousAccount(t *testing.T) {
	google := &fakeGoogle{grant: Grant{RefreshToken: "new", Account: "new@example.invalid"}}
	grants := &fakeGrants{grant: &Grant{RefreshToken: "old", Account: "old@example.invalid"}}

	status, err := newTestService(google, grants).Connect(context.Background(), validConnect())
	if err != nil {
		t.Fatal(err)
	}
	if status.Account != "new@example.invalid" || grants.grant.RefreshToken != "new" {
		t.Errorf("the new account should be stored, got %+v", grants.grant)
	}
	if len(google.revoked) != 1 || google.revoked[0] != "old" {
		t.Errorf("the previous grant should be revoked, revoked %v", google.revoked)
	}
}

func TestConnectStillSucceedsWhenRevokingThePreviousAccountFails(t *testing.T) {
	google := &fakeGoogle{grant: Grant{RefreshToken: "new"}, revokeErr: ErrGoogleUnreachable}
	grants := &fakeGrants{grant: &Grant{RefreshToken: "old"}}

	if _, err := newTestService(google, grants).Connect(context.Background(), validConnect()); err != nil {
		t.Fatalf("a failed revoke must not stand in the way of the new account, got %v", err)
	}
	if grants.grant.RefreshToken != "new" {
		t.Error("the new grant should still be stored")
	}
}

func TestConnectKeepsNothingWhenGoogleRefuses(t *testing.T) {
	google := &fakeGoogle{err: ErrGoogleRefused}
	grants := &fakeGrants{}

	_, err := newTestService(google, grants).Connect(context.Background(), validConnect())
	if !errors.Is(err, ErrGoogleRefused) {
		t.Errorf("Google's refusal should come through, got %v", err)
	}
	if grants.grant != nil {
		t.Error("nothing should be stored for a refused exchange")
	}
}

// Disconnecting is the operator saying "stop using this account". That part is
// ours entirely, so it happens whether or not Google heard the revoke.
func TestDisconnectForgetsTheGrantEvenWhenGoogleDoesNotAnswer(t *testing.T) {
	google := &fakeGoogle{revokeErr: ErrGoogleUnreachable}
	grants := &fakeGrants{grant: &Grant{RefreshToken: "refresh"}}

	if err := newTestService(google, grants).Disconnect(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !grants.deleted || len(google.revoked) != 1 {
		t.Errorf("expected a revoke attempt and a deletion, got revoked=%v deleted=%v",
			google.revoked, grants.deleted)
	}
}

func TestDisconnectingNothingIsNotAnError(t *testing.T) {
	google := &fakeGoogle{}
	if err := newTestService(google, &fakeGrants{}).Disconnect(context.Background()); err != nil {
		t.Errorf("disconnecting with no account should succeed, got %v", err)
	}
	if len(google.revoked) != 0 {
		t.Error("there was nothing to revoke")
	}
}

func TestAccessTokenNeedsAConnectedAccount(t *testing.T) {
	_, err := newTestService(&fakeGoogle{}, &fakeGrants{}).AccessToken(context.Background())
	if !errors.Is(err, ErrNotConnected) {
		t.Errorf("expected not connected, got %v", err)
	}
}

func validConnect() ConnectRequest {
	return ConnectRequest{
		Code:         "code",
		CodeVerifier: strings.Repeat("v", minVerifierLength),
		RedirectURI:  "https://panel.example.invalid/api/backups/google/callback",
	}
}
