// Package secretgen mints credentials, so nobody has to invent one.
//
// The only implementation. The panel's password fields reach it through a server
// action and the assistant reaches it as an ordinary API call, which is the point
// of it being here: rejection sampling written twice is rejection sampling that
// will disagree with itself eventually.
//
// The value travels in the response, and for the assistant that means it lands in
// a model's context and in whatever the runtime remembers. Worth knowing; not
// worth a second implementation to avoid.
package secretgen

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// Shape is what kind of value to make.
type Shape string

// The four shapes worth offering.
const (
	// ShapePassword is letters, digits and symbols, for a database role or an
	// application login.
	ShapePassword Shape = "password"
	// ShapeAlphanumeric is letters and digits only, for a value that will be
	// pasted into a connection string or a shell.
	ShapeAlphanumeric Shape = "alphanumeric"
	// ShapeHex is 256 bits, hex encoded, for an API token or a signing key.
	ShapeHex Shape = "hex"
	// ShapeBase64 is 256 bits, base64 encoded — the AUTH_SECRET shape, which is
	// what `npx auth secret` produces.
	ShapeBase64 Shape = "base64"
)

const (
	letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	digits  = "0123456789"
	// symbols leaves out quotes, backslashes, backticks and dollar signs on
	// purpose. This value ends up in a YAML values file, a connection string, an
	// environment variable and sooner or later a shell, and every one of those has
	// a different opinion about those characters. A password that needs escaping
	// is a password that will be transcribed wrongly.
	symbols = "!#%*+-.:=?@^_~"
)

// byteValues is how many values a byte has. Named because the rejection limit
// below is arithmetic about that fact rather than an arbitrary constant.
const byteValues = 256

// batchSlack is how many extra bytes each draw asks for, to cover the ones
// rejection will discard. Roughly a fifth of a draw is thrown away over these
// alphabets, so a little slack turns the common case into one call instead of
// two. The loop does not depend on it being enough.
const batchSlack = 8

// The bounds on a sized shape, and the fixed size of a token.
const (
	DefaultLength = 24
	MinLength     = 12
	MaxLength     = 128
	// tokenBytes is 256 bits, which is the requirement rather than a default —
	// which is why hex and base64 do not take a length.
	tokenBytes = 32
)

// Generate makes one value.
//
// Length is ignored for hex and base64, deliberately: "32 bytes" is what
// AUTH_SECRET means, and honouring a length there would let a caller ask for a
// signing key of eight.
func Generate(shape Shape, length int) (string, error) {
	switch shape {
	case ShapeHex, ShapeBase64:
		bytes := make([]byte, tokenBytes)
		if _, err := rand.Read(bytes); err != nil {
			return "", fmt.Errorf("read randomness: %w", err)
		}
		if shape == ShapeHex {
			return hex.EncodeToString(bytes), nil
		}
		return base64.StdEncoding.EncodeToString(bytes), nil

	case ShapePassword, ShapeAlphanumeric:
		if length < MinLength || length > MaxLength {
			return "", fmt.Errorf("%w: length must be between %d and %d",
				ErrInvalidRequest, MinLength, MaxLength)
		}
		alphabet := letters + digits
		if shape == ShapePassword {
			alphabet += symbols
		}
		return fromAlphabet(alphabet, length)

	default:
		return "", fmt.Errorf("%w: shape must be %s, %s, %s or %s",
			ErrInvalidRequest, ShapePassword, ShapeAlphanumeric, ShapeHex, ShapeBase64)
	}
}

// fromAlphabet draws length characters uniformly from alphabet.
//
// Rejection sampling, and it is the whole reason this is not three lines.
// `b % len(alphabet)` is the obvious mapping and it is biased: 256 is not a
// multiple of any alphabet used here, so the first 256 % len(alphabet) characters
// come up more often than the rest. Bytes at or above the largest multiple are
// thrown away and redrawn instead.
//
// Drawn in batches, because a rejection rate near a fifth would otherwise mean a
// syscall per discarded byte — and the loop keeps going until it has enough, so a
// run of rejections cannot cut the result short. That last part is the failure
// that would be invisible: a password shorter than asked for still looks like a
// password.
func fromAlphabet(alphabet string, length int) (string, error) {
	// An int, not a byte. An alphabet of 64 makes this exactly byteValues, which
	// as a byte is 0 — every draw would be rejected and the loop below would never
	// end. Neither alphabet here is 64 today, which is the kind of fact that
	// stops being true when somebody adds a symbol.
	limit := byteValues / len(alphabet) * len(alphabet)
	out := make([]byte, 0, length)

	for len(out) < length {
		batch := make([]byte, length-len(out)+batchSlack)
		if _, err := rand.Read(batch); err != nil {
			return "", fmt.Errorf("read randomness: %w", err)
		}
		for _, b := range batch {
			if len(out) == length {
				break
			}
			if int(b) >= limit {
				continue
			}
			out = append(out, alphabet[int(b)%len(alphabet)])
		}
	}

	return string(out), nil
}
