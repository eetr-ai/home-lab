package postgres

import (
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
)

var verifierShape = regexp.MustCompile(
	`^SCRAM-SHA-256\$(\d+):([A-Za-z0-9+/]+={0,2})\$([A-Za-z0-9+/]+={0,2}):([A-Za-z0-9+/]+={0,2})$`)

func TestScramVerifierShape(t *testing.T) {
	verifier, err := scramVerifier("correct horse battery staple")
	if err != nil {
		t.Fatalf("scramVerifier() = %v", err)
	}

	groups := verifierShape.FindStringSubmatch(verifier)
	if groups == nil {
		t.Fatalf("verifier %q does not match the format PostgreSQL stores", verifier)
	}
	if groups[1] != "4096" {
		t.Errorf("iterations = %s, want 4096 to match PostgreSQL's own", groups[1])
	}

	salt, err := base64.StdEncoding.DecodeString(groups[2])
	if err != nil || len(salt) != scramSaltLength {
		t.Errorf("salt = %d bytes, want %d", len(salt), scramSaltLength)
	}
	for i, name := range []string{"stored key", "server key"} {
		key, decodeErr := base64.StdEncoding.DecodeString(groups[3+i])
		if decodeErr != nil || len(key) != 32 {
			t.Errorf("%s = %d bytes, want 32", name, len(key))
		}
	}
}

// The plaintext must not survive anywhere in the value handed to the server —
// that is the entire point of computing a verifier rather than sending a password.
func TestScramVerifierDoesNotContainThePassword(t *testing.T) {
	const password = "hunter2-hunter2-hunter2"
	verifier, err := scramVerifier(password)
	if err != nil {
		t.Fatalf("scramVerifier() = %v", err)
	}
	if strings.Contains(verifier, password) {
		t.Fatalf("the verifier contains the plaintext password")
	}
}

// A fresh salt every time, so two roles given the same password do not store the same
// verifier — which would otherwise let anyone reading the catalog see that they
// match.
func TestScramVerifierSaltsEachCall(t *testing.T) {
	first, err := scramVerifier("same password")
	if err != nil {
		t.Fatalf("scramVerifier() = %v", err)
	}
	second, err := scramVerifier("same password")
	if err != nil {
		t.Fatalf("scramVerifier() = %v", err)
	}
	if first == second {
		t.Fatal("two calls produced the same verifier; the salt is not random")
	}
}

func TestScramVerifierWithSaltIsDeterministic(t *testing.T) {
	salt := []byte("0123456789abcdef")

	first, err := scramVerifierWithSalt("password", salt)
	if err != nil {
		t.Fatalf("scramVerifierWithSalt() = %v", err)
	}
	second, err := scramVerifierWithSalt("password", salt)
	if err != nil {
		t.Fatalf("scramVerifierWithSalt() = %v", err)
	}
	if first != second {
		t.Error("the same password and salt produced different verifiers")
	}

	different, err := scramVerifierWithSalt("password", []byte("fedcba9876543210"))
	if err != nil {
		t.Fatalf("scramVerifierWithSalt() = %v", err)
	}
	if first == different {
		t.Error("a different salt produced the same verifier")
	}
}

// RFC 5802 normalization, which PostgreSQL also applies. Without it a password
// containing anything but ASCII produces a verifier the server will not
// authenticate against, and the failure looks like a wrong password.
func TestScramVerifierNormalizesThePassword(t *testing.T) {
	salt := []byte("0123456789abcdef")

	// U+00A0 NO-BREAK SPACE maps to an ordinary space under OpaqueString, so
	// these two passwords must derive the same verifier.
	withNoBreakSpace, err := scramVerifierWithSalt("a\u00a0b", salt)
	if err != nil {
		t.Fatalf("scramVerifierWithSalt() = %v", err)
	}
	withPlainSpace, err := scramVerifierWithSalt("a b", salt)
	if err != nil {
		t.Fatalf("scramVerifierWithSalt() = %v", err)
	}
	if withNoBreakSpace != withPlainSpace {
		t.Error("a no-break space was not normalized to a space before hashing")
	}

	// And a genuinely different password must still differ, so the assertion
	// above is about normalization rather than about everything collapsing.
	different, err := scramVerifierWithSalt("a\tb", salt)
	if err == nil && different == withPlainSpace {
		t.Error("an unrelated password derived the same verifier")
	}
}
