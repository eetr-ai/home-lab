package auth

import "errors"

// errNoIssuer reports a verifier asked for without an identity provider to check
// against. It is a configuration mistake rather than a runtime condition, and it
// stops the process at startup rather than failing every request later.
var errNoIssuer = errors.New("no openid issuer configured")
