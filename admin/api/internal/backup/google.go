package backup

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// Scopes is what the panel asks Google for, and the whole of what it can reach.
//
// drive.file is the narrowest Drive scope there is: the panel sees the files it
// created and the folders an operator picked for it in Google's Picker, and
// nothing else in the account. It is also a non-sensitive scope, which is why the
// consent screen needs no verification. openid and email are only there so the
// panel can say which account is connected.
var Scopes = []string{
	"https://www.googleapis.com/auth/drive.file",
	"openid",
	"email",
}

// Google's endpoints. Variables rather than constants so a test can point them at
// an httptest server; nothing in production changes them.
var (
	googleAuthURL   = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL  = "https://oauth2.googleapis.com/token" //nolint:gosec // a URL, not a credential
	googleRevokeURL = "https://oauth2.googleapis.com/revoke"
)

// googleTimeout bounds every call this process makes to Google.
//
// The panel's client gives up at twenty seconds, so a slower answer than this
// would land on a request nobody is waiting for anymore.
const googleTimeout = 15 * time.Second

// Grant is what connecting an account produces: the long-lived credential and the
// account it belongs to.
type Grant struct {
	RefreshToken string
	Account      string
}

// AccessToken is a short-lived credential the browser needs for Google's Picker.
type AccessToken struct {
	AccessToken string    `json:"accessToken"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// GoogleClient talks to Google's OAuth endpoints with this lab's client.
type GoogleClient struct {
	config     oauth2.Config
	httpClient *http.Client
}

// NewGoogleClient builds the client. It contacts nothing.
func NewGoogleClient(clientID, clientSecret string) *GoogleClient {
	return &GoogleClient{
		config: oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       Scopes,
			Endpoint: oauth2.Endpoint{
				AuthURL:  googleAuthURL,
				TokenURL: googleTokenURL,
				// Google takes the client credentials in the body. Saying so avoids
				// the library's auto-detection, which spends a failed request
				// finding out.
				AuthStyle: oauth2.AuthStyleInParams,
			},
		},
		httpClient: &http.Client{Timeout: googleTimeout},
	}
}

// Exchange trades the authorization code from the consent screen for a grant.
//
// The redirect URI must be the one the browser was sent to Google with, byte for
// byte; Google refuses the exchange otherwise. It is the panel's to know, which
// is why it arrives with the request rather than living in this process's config.
func (c *GoogleClient) Exchange(ctx context.Context, code, verifier, redirectURI string) (Grant, error) {
	config := c.config
	config.RedirectURL = redirectURI

	token, err := config.Exchange(c.context(ctx), code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Grant{}, googleError("exchange the authorization code", err)
	}

	// Without offline access there is nothing a scheduled job could use. Google
	// leaves it out when the account already granted this client and the consent
	// screen was skipped, which the panel prevents by always asking with
	// prompt=consent — so reaching this is worth a sentence that says what to do.
	if token.RefreshToken == "" {
		return Grant{}, fmt.Errorf("%w: Google did not hand out offline access; remove the "+
			"panel from your Google account's third-party access and connect again", ErrGoogleRefused)
	}

	return Grant{RefreshToken: token.RefreshToken, Account: accountOf(token)}, nil
}

// AccessToken mints a fresh access token from the stored refresh token.
func (c *GoogleClient) AccessToken(ctx context.Context, refreshToken string) (AccessToken, error) {
	token, err := c.config.TokenSource(c.context(ctx), &oauth2.Token{RefreshToken: refreshToken}).Token()
	if err != nil {
		return AccessToken{}, googleError("refresh the access token", err)
	}
	return AccessToken{AccessToken: token.AccessToken, ExpiresAt: token.Expiry}, nil
}

// Drive returns a Drive client acting with the stored grant.
//
// Its HTTP client has no timeout of its own, deliberately unlike every other call
// here: an upload of a large dump legitimately takes longer than any fixed bound,
// and it is the context — the run's deadline, or the caller's request — that ends
// it. The token refresh rides the same client.
func (c *GoogleClient) Drive(ctx context.Context, refreshToken string) *DriveClient {
	unbounded := context.WithValue(ctx, oauth2.HTTPClient, &http.Client{})
	source := c.config.TokenSource(unbounded, &oauth2.Token{RefreshToken: refreshToken})
	return &DriveClient{httpClient: oauth2.NewClient(unbounded, source)}
}

// ListBackups lists what a schedule has kept in its folder.
func (c *GoogleClient) ListBackups(ctx context.Context, refreshToken, folderID, schedule string) ([]File, error) {
	return c.Drive(ctx, refreshToken).List(ctx, folderID, schedule)
}

// Revoke tells Google to forget the grant, so a copy of the token that leaked
// anywhere stops working too.
func (c *GoogleClient) Revoke(ctx context.Context, refreshToken string) error {
	form := url.Values{"token": {refreshToken}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, googleRevokeURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build the revoke request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("%w: revoke the grant: %w", ErrGoogleUnreachable, err)
	}
	defer func() { _ = response.Body.Close() }()

	// A 400 here means Google no longer knows the token — revoked already, or
	// expired. Forgetting something already forgotten is success.
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("%w: revoking the grant answered %d", ErrGoogleRefused, response.StatusCode)
	}
	return nil
}

// context hands the oauth2 library this client's bounded HTTP client, which is
// the only way that library accepts one.
func (c *GoogleClient) context(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, c.httpClient)
}

// googleError sorts an oauth2 failure into "Google said no" and "Google never
// answered", because the two have different fixes and different statuses.
func googleError(what string, err error) error {
	var retrieve *oauth2.RetrieveError
	if errors.As(err, &retrieve) {
		// A 5xx is Google failing, not Google saying no, and the fix is to try again
		// rather than to reconnect — so it is unreachable, whatever the body says.
		if retrieve.Response != nil && retrieve.Response.StatusCode >= http.StatusInternalServerError {
			return fmt.Errorf("%w: %s: Google answered %d", ErrGoogleUnreachable, what,
				retrieve.Response.StatusCode)
		}
		// Google's own error code (invalid_grant, invalid_client) is kept: it is the
		// one word that tells an operator whether to reconnect or fix the client.
		return fmt.Errorf("%w: %s: %s", ErrGoogleRefused, what, retrieve.ErrorCode)
	}
	return fmt.Errorf("%w: %s: %w", ErrGoogleUnreachable, what, err)
}

// accountOf reads the email address out of the ID token Google returned alongside
// the grant.
//
// The signature is not checked, and that is the OpenID Connect rule rather than a
// shortcut: a token received directly from the token endpoint over TLS may be
// trusted as-is (Core 1.0, §3.1.3.7). It is only ever used as a label for the
// panel to show, never as an identity anything is decided on.
func accountOf(token *oauth2.Token) string {
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return ""
	}
	// header.payload.signature
	const jwtParts = 3
	parts := strings.Split(raw, ".")
	if len(parts) != jwtParts {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}

	var claims struct {
		Email string `json:"email"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	return claims.Email
}
