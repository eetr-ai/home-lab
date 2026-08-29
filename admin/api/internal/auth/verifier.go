package auth

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
)

// OIDCVerifier verifies access tokens against an OpenID Connect provider,
// discovering its signing keys from the provider's own metadata.
type OIDCVerifier struct {
	verifier *oidc.IDTokenVerifier
}

// NewOIDCVerifier reads the provider's discovery document and builds a verifier
// from it.
//
// The issuer is kept exactly as configured, trailing slash and all: it is an
// identifier rather than a path, and some providers really do publish one that
// ends in a slash. Normalizing it here would silently fail every issuer check
// against those.
//
// The audience is the value tokens must name in `aud`. Leave it empty only when
// the provider cannot be made to issue an audience — the check is what stops a
// token minted for a different application being replayed at this one.
func NewOIDCVerifier(ctx context.Context, issuer, audience string) (*OIDCVerifier, error) {
	if issuer == "" {
		return nil, errNoIssuer
	}

	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("discover the openid provider at %q: %w", issuer, err)
	}

	config := &oidc.Config{
		ClientID: audience,
		// Stated rather than left to the default. go-oidc happens to default to
		// RS256 today, so this changes nothing now — it pins the property so a
		// library default that widens later cannot quietly make an
		// algorithm-confusion attack possible, and it puts the constraint where
		// someone reading this file can see it.
		SupportedSigningAlgs: []string{oidc.RS256},
	}
	if audience == "" {
		config.SkipClientIDCheck = true
	}

	return &OIDCVerifier{verifier: provider.Verifier(config)}, nil
}

// Verify checks the token's signature, issuer, audience, and expiry, and returns
// who it belongs to.
func (v *OIDCVerifier) Verify(ctx context.Context, rawToken string) (Subject, error) {
	token, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return Subject{}, fmt.Errorf("verify the bearer token: %w", err)
	}

	// The subject is the identifier; everything else here is optional. A token
	// with no email and no scope claim is still perfectly valid, so a failure to
	// read the claims is not a failure to authenticate.
	// Every field is raw. A provider that spells `scope` as an array rather than
	// as a space-delimited string is within its rights, and decoding it into a
	// string fails the whole struct — which would leave this token looking like
	// one that named no scopes at all, and a scopeless token is unrestricted.
	// Reading it as an authorization decision is exactly how that becomes a
	// bypass, so no claim's shape is allowed to decide another claim's fate.
	var claims struct {
		Email json.RawMessage `json:"email"`
		Scope json.RawMessage `json:"scope"`
		// Both spellings: client_id is RFC 9068's, azp is OpenID Connect's, and
		// providers disagree about which they emit.
		ClientID json.RawMessage `json:"client_id"`
		Azp      json.RawMessage `json:"azp"`
		Scp      json.RawMessage `json:"scp"`
	}
	_ = token.Claims(&claims)

	var email string
	_ = json.Unmarshal(claims.Email, &email)

	return Subject{
		ID:       token.Subject,
		Email:    email,
		ClientID: stringClaim(claims.ClientID, claims.Azp),
		Scopes:   parseScopes(claims.Scope, claims.Scp),
	}, nil
}

// stringClaim reads the first of these claims that carries a string.
//
// Each is decoded on its own, for the same reason every claim above is a
// json.RawMessage: one claim arriving in an unexpected shape must not decide
// another claim's fate.
func stringClaim(candidates ...json.RawMessage) string {
	for _, candidate := range candidates {
		var value string
		if err := json.Unmarshal(candidate, &value); err == nil && value != "" {
			return value
		}
	}
	return ""
}
