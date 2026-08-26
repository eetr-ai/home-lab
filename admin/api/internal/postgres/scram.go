package postgres

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"golang.org/x/text/secure/precis"
)

// scramIterations is the PBKDF2 work factor. 4096 is what PostgreSQL itself uses
// for `password_encryption = scram-sha-256`, and matching it keeps a role created
// here indistinguishable from one created with ALTER ROLE ... PASSWORD.
const scramIterations = 4096

// scramSaltLength is 16 bytes, again matching PostgreSQL's own.
const scramSaltLength = 16

// scramVerifier turns a password into the verifier PostgreSQL stores.
//
// This is why the plaintext password never appears in a statement. `CREATE ROLE
// x PASSWORD 'secret'` sends the password to the server, where it is visible in
// pg_stat_activity while the statement runs and is written to the server log
// whenever log_statement is on — neither of which this panel controls. Handing
// over the verifier instead means the server learns a value that cannot be
// replayed as a password, which is exactly what psql's \password does.
func scramVerifier(password string) (string, error) {
	salt := make([]byte, scramSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate a password salt: %w", err)
	}
	return scramVerifierWithSalt(password, salt)
}

// scramVerifierWithSalt is the pure half, so the derivation can be tested without
// depending on what the random source returned.
func scramVerifierWithSalt(password string, salt []byte) (string, error) {
	// RFC 5802 requires the password to be normalized before hashing, and
	// PostgreSQL applies the same rule. Skipping it produces a verifier that
	// works for ASCII and silently fails to authenticate for anything else.
	prepared, err := precis.OpaqueString.String(password)
	if err != nil {
		return "", fmt.Errorf("%w: the password cannot be normalized for storage", ErrInvalidName)
	}

	saltedPassword, err := pbkdf2.Key(sha256.New, prepared, salt, scramIterations, sha256.Size)
	if err != nil {
		return "", fmt.Errorf("derive the password key: %w", err)
	}

	clientKey := scramHMAC(saltedPassword, "Client Key")
	storedKey := sha256.Sum256(clientKey)
	serverKey := scramHMAC(saltedPassword, "Server Key")

	encode := base64.StdEncoding.EncodeToString
	return fmt.Sprintf("SCRAM-SHA-256$%d:%s$%s:%s",
		scramIterations, encode(salt), encode(storedKey[:]), encode(serverKey)), nil
}

func scramHMAC(key []byte, message string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(message))
	return mac.Sum(nil)
}
