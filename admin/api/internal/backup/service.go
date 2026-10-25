// Package backup schedules database backups into Google Drive.
//
// Two halves, both in the cluster. The connection is one Google account, whose
// grant lives in a Secret in the panel's namespace. A schedule is a CronJob there
// too: its pods dump the chosen databases and upload them to a folder the operator
// picked, then trash whatever is past the number of backups they asked to keep.
//
// Nothing is stored anywhere else. Asking what is scheduled is listing CronJobs;
// asking what ran is listing their Jobs; asking what was kept is listing the
// folder. Every answer comes from the thing that actually holds it.
package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
)

// Google is the part of Google's OAuth this slice needs.
//
// Declared here, where it is consumed, so the service is tested against a fake and
// never against accounts.google.com.
type Google interface {
	Exchange(ctx context.Context, code, verifier, redirectURI string) (Grant, error)
	AccessToken(ctx context.Context, refreshToken string) (AccessToken, error)
	Revoke(ctx context.Context, refreshToken string) error
}

// GrantStore keeps the connected account.
type GrantStore interface {
	ReadGrant(ctx context.Context) (Grant, error)
	WriteGrant(ctx context.Context, grant Grant) error
	DeleteGrant(ctx context.Context) error
}

// GoogleConfig is this lab's OAuth client and Picker, from the chart.
type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	PickerAPIKey string
	PickerAppID  string
}

// Configured reports whether there is an OAuth client to connect with.
func (c GoogleConfig) Configured() bool {
	return c.ClientID != "" && c.ClientSecret != ""
}

// PKCE verifiers are 43 to 128 characters (RFC 7636 §4.1). Anything else is not a
// verifier the panel made, and sending it to Google only buys a vaguer refusal.
const (
	minVerifierLength = 43
	maxVerifierLength = 128
)

// Service is the backups domain.
type Service struct {
	// google is nil when this lab has no OAuth client, and every Google route
	// then answers ErrNotConfigured rather than calling through a nil.
	google Google
	grants GrantStore
	config GoogleConfig
	logger *slog.Logger
}

// NewService builds the service. A nil google is supported: it is a lab that has
// not set up an OAuth client yet.
func NewService(google Google, grants GrantStore, config GoogleConfig, logger *slog.Logger) *Service {
	return &Service{google: google, grants: grants, config: config, logger: logger}
}

// GoogleStatus reports whether Drive is set up and connected, and what the panel
// needs to open the Picker.
func (s *Service) GoogleStatus(ctx context.Context) (GoogleStatus, error) {
	if s.google == nil {
		return GoogleStatus{}, nil
	}

	ret := GoogleStatus{
		Configured:   true,
		ClientID:     s.config.ClientID,
		Scopes:       Scopes,
		PickerAPIKey: s.config.PickerAPIKey,
		PickerAppID:  s.config.PickerAppID,
	}

	grant, err := s.grants.ReadGrant(ctx)
	if errors.Is(err, ErrNotConnected) {
		return ret, nil
	}
	if err != nil {
		return GoogleStatus{}, err
	}

	ret.Connected = true
	ret.Account = grant.Account
	return ret, nil
}

// Connect finishes the OAuth flow and keeps the grant, replacing any account that
// was connected before.
func (s *Service) Connect(ctx context.Context, request ConnectRequest) (GoogleStatus, error) {
	if s.google == nil {
		return GoogleStatus{}, ErrNotConfigured
	}
	if err := request.validate(); err != nil {
		return GoogleStatus{}, err
	}

	grant, err := s.google.Exchange(ctx, request.Code, request.CodeVerifier, request.RedirectURI)
	if err != nil {
		return GoogleStatus{}, fmt.Errorf("connect Google Drive: %w", err)
	}

	previous, previousErr := s.grants.ReadGrant(ctx)

	// Written before anything is revoked, so a failed write leaves the old grant
	// working rather than leaving no grant at all.
	if err := s.grants.WriteGrant(ctx, grant); err != nil {
		return GoogleStatus{}, err
	}

	// Replacing an account leaves the old grant alive at Google unless somebody
	// tells it otherwise. Only for a *different* account, though: Google revokes
	// a grant per account and client, not per token, so revoking the previous
	// token of the same account would revoke the one just issued along with it.
	// An account Google did not name is treated as possibly the same one, which
	// errs towards a stale grant over a dead connection.
	//
	// Best effort: the new account is what was asked for, and a revoke that fails
	// must not stand in the way of it.
	if previousErr == nil && previous.Account != "" && grant.Account != "" &&
		previous.Account != grant.Account {
		if err := s.google.Revoke(ctx, previous.RefreshToken); err != nil {
			s.logger.Warn("could not revoke the previous Google grant", slog.Any("error", err))
		}
	}
	s.logger.Info("connected Google Drive", slog.String("account", grant.Account))
	return s.GoogleStatus(ctx)
}

// Disconnect revokes the grant at Google and forgets it here.
//
// Forgetting happens even when revoking fails. An operator who pressed disconnect
// wants the panel to stop using the account, and that part is entirely ours; a
// grant Google still honours is then theirs to remove from their account page,
// which is said in the log.
func (s *Service) Disconnect(ctx context.Context) error {
	if s.google == nil {
		return ErrNotConfigured
	}

	grant, err := s.grants.ReadGrant(ctx)
	if errors.Is(err, ErrNotConnected) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := s.google.Revoke(ctx, grant.RefreshToken); err != nil {
		s.logger.Warn("could not revoke the Google grant; remove the panel from the account's "+
			"third-party access to finish disconnecting", slog.Any("error", err))
	}
	return s.grants.DeleteGrant(ctx)
}

// AccessToken mints a short-lived token for the browser's Picker.
//
// This is a Drive credential leaving the API, and that is the Picker's design
// rather than a leak: it runs in the operator's browser and talks to Google
// directly. drive.file bounds what the token reaches, and it expires in an hour.
func (s *Service) AccessToken(ctx context.Context) (AccessToken, error) {
	if s.google == nil {
		return AccessToken{}, ErrNotConfigured
	}
	grant, err := s.grants.ReadGrant(ctx)
	if err != nil {
		return AccessToken{}, err
	}
	token, err := s.google.AccessToken(ctx, grant.RefreshToken)
	if err != nil {
		return AccessToken{}, fmt.Errorf("open the folder picker: %w", err)
	}
	return token, nil
}

// validate checks the shape of a connect request before Google is asked anything.
func (r ConnectRequest) validate() error {
	if strings.TrimSpace(r.Code) == "" {
		return fmt.Errorf("%w: an authorization code is required", ErrInvalid)
	}
	if n := len(r.CodeVerifier); n < minVerifierLength || n > maxVerifierLength {
		return fmt.Errorf("%w: the code verifier must be %d to %d characters, and is %d",
			ErrInvalid, minVerifierLength, maxVerifierLength, n)
	}

	// Google checks the redirect against what the client registered, so this is
	// not the security check. It is here so a malformed one is a 400 that says so,
	// rather than an invalid_grant that says nothing.
	redirect, err := url.Parse(r.RedirectURI)
	if err != nil || redirect.Host == "" || (redirect.Scheme != "https" && redirect.Scheme != "http") {
		return fmt.Errorf("%w: the redirect URI must be an absolute http(s) URL", ErrInvalid)
	}
	return nil
}
