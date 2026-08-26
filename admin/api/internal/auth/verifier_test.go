package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const testAudience = "home-lab-admin"

// fakeIssuer is an OpenID provider that exists only for this test: it publishes a
// discovery document and a JWKS, and hands back the key to mint tokens with.
type fakeIssuer struct {
	url    string
	key    *rsa.PrivateKey
	keyID  string
	server *httptest.Server
}

func newFakeIssuer(t *testing.T) *fakeIssuer {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}

	issuer := &fakeIssuer{key: key, keyID: "test-key-1"}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuer.url,
			"jwks_uri":                              issuer.url + "/jwks",
			"authorization_endpoint":                issuer.url + "/authorize",
			"token_endpoint":                        issuer.url + "/token",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})

	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{
			Keys: []jose.JSONWebKey{{
				Key:       key.Public(),
				KeyID:     issuer.keyID,
				Algorithm: string(jose.RS256),
				Use:       "sig",
			}},
		})
	})

	issuer.server = httptest.NewServer(mux)
	issuer.url = issuer.server.URL
	t.Cleanup(issuer.server.Close)
	return issuer
}

type claims struct {
	jwt.Claims
	Email string `json:"email,omitempty"`
}

// signWith mints a token with an arbitrary key, so a test can present one this
// issuer never signed.
func (f *fakeIssuer) signWith(t *testing.T, key *rsa.PrivateKey, c claims) string {
	t.Helper()
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader(jose.HeaderKey("kid"), f.keyID),
	)
	if err != nil {
		t.Fatalf("build signer: %v", err)
	}
	raw, err := jwt.Signed(signer).Claims(c).Serialize()
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return raw
}

func (f *fakeIssuer) validClaims() claims {
	now := time.Now()
	return claims{
		Claims: jwt.Claims{
			Issuer:   f.url,
			Subject:  "user-abc",
			Audience: jwt.Audience{testAudience},
			IssuedAt: jwt.NewNumericDate(now),
			Expiry:   jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Email: "operator@example.invalid",
	}
}

func TestOIDCVerifier(t *testing.T) {
	issuer := newFakeIssuer(t)
	ctx := context.Background()

	verifier, err := NewOIDCVerifier(ctx, issuer.url, testAudience)
	if err != nil {
		t.Fatalf("build verifier: %v", err)
	}

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate second key: %v", err)
	}

	tests := []struct {
		name   string
		token  func() string
		wantOK bool
	}{
		{
			name:   "a token this issuer signed for this audience",
			token:  func() string { return issuer.signWith(t, issuer.key, issuer.validClaims()) },
			wantOK: true,
		},
		{
			// The whole point of the audience check: a token minted for another
			// application must not be replayable here.
			name: "a token for a different audience",
			token: func() string {
				c := issuer.validClaims()
				c.Audience = jwt.Audience{"some-other-application"}
				return issuer.signWith(t, issuer.key, c)
			},
		},
		{
			name: "an expired token",
			token: func() string {
				c := issuer.validClaims()
				c.Expiry = jwt.NewNumericDate(time.Now().Add(-time.Minute))
				return issuer.signWith(t, issuer.key, c)
			},
		},
		{
			name: "a token naming a different issuer",
			token: func() string {
				c := issuer.validClaims()
				c.Issuer = "https://somewhere.else.invalid"
				return issuer.signWith(t, issuer.key, c)
			},
		},
		{
			// Signed correctly, but by a key the issuer does not publish.
			name:  "a token signed by an unknown key",
			token: func() string { return issuer.signWith(t, otherKey, issuer.validClaims()) },
		},
		{
			name:  "not a token at all",
			token: func() string { return "not.a.jwt" },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			subject, err := verifier.Verify(ctx, test.token())
			if test.wantOK {
				if err != nil {
					t.Fatalf("Verify() returned %v, want a verified subject", err)
				}
				if subject.ID != "user-abc" {
					t.Errorf("subject.ID = %q, want %q", subject.ID, "user-abc")
				}
				if subject.Email != "operator@example.invalid" {
					t.Errorf("subject.Email = %q, want the email claim", subject.Email)
				}
				return
			}
			if err == nil {
				t.Fatalf("Verify() accepted %s", test.name)
			}
		})
	}
}

// Algorithm confusion: a token signed with HMAC, using the issuer's public key as
// the shared secret, verifies against a verifier willing to try HMAC at all.
//
// This passes with or without the explicit RS256 pin in the verifier, because
// go-oidc already defaults to RS256. That is the point of asserting the behavior
// rather than the setting: it holds whichever of the two is providing it, and it
// fails if either stops.
func TestOIDCVerifierRejectsSymmetricallySignedTokens(t *testing.T) {
	issuer := newFakeIssuer(t)
	ctx := context.Background()

	verifier, err := NewOIDCVerifier(ctx, issuer.url, testAudience)
	if err != nil {
		t.Fatalf("build verifier: %v", err)
	}

	publicKey, err := issuer.key.N.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal public modulus: %v", err)
	}
	// go-jose requires at least 256 bits of key material for HS256.
	secret := make([]byte, 0, len(publicKey)+32)
	secret = append(secret, publicKey...)
	secret = append(secret, make([]byte, 32)...)

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.HS256, Key: secret},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader(jose.HeaderKey("kid"), issuer.keyID),
	)
	if err != nil {
		t.Fatalf("build hmac signer: %v", err)
	}
	raw, err := jwt.Signed(signer).Claims(issuer.validClaims()).Serialize()
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	if _, err := verifier.Verify(ctx, raw); err == nil {
		t.Fatal("Verify() accepted an HS256-signed token")
	}
}

func TestNewOIDCVerifierRequiresAnIssuer(t *testing.T) {
	if _, err := NewOIDCVerifier(context.Background(), "", testAudience); err == nil {
		t.Fatal("NewOIDCVerifier() accepted an empty issuer")
	}
}
