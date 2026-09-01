package secretgen

import (
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestGenerateMakesTheLengthAskedFor(t *testing.T) {
	for _, length := range []int{MinLength, 20, DefaultLength, 64, MaxLength} {
		value, err := Generate(ShapePassword, length)
		if err != nil {
			t.Fatalf("Generate(password, %d) error = %v", length, err)
		}
		if len(value) != length {
			t.Errorf("len = %d, want %d", len(value), length)
		}
	}
}

// 256 bits, which is the requirement rather than a default. The length is ignored
// on purpose, and asserting that is what stops a caller shortening a signing key.
func TestGenerateIgnoresLengthForTheTokenShapes(t *testing.T) {
	hexValue, err := Generate(ShapeHex, MinLength)
	if err != nil {
		t.Fatalf("Generate(hex) error = %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(hexValue) {
		t.Errorf("hex = %q, want 64 hex characters", hexValue)
	}

	base64Value, err := Generate(ShapeBase64, MinLength)
	if err != nil {
		t.Fatalf("Generate(base64) error = %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(base64Value)
	if err != nil {
		t.Fatalf("the base64 shape did not decode: %v", err)
	}
	if len(decoded) != tokenBytes {
		t.Errorf("base64 decoded to %d bytes, want %d", len(decoded), tokenBytes)
	}
}

// A password goes into a YAML values file, a connection string, an environment
// variable and sooner or later a shell. Anything needing an escape in one of
// those will be transcribed wrongly, and it will be transcribed wrongly once,
// quietly, by somebody in a hurry.
func TestPasswordsHoldNothingThatNeedsEscaping(t *testing.T) {
	const forbidden = "'\"`$\\{}[]()<>|&;, \t\n"

	// Many draws, because the alphabet is sampled and one value proves nothing
	// about the character that would have been drawn next.
	for range 200 {
		value, err := Generate(ShapePassword, MaxLength)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if strings.ContainsAny(value, forbidden) {
			t.Fatalf("a password held a character that needs escaping: %q", value)
		}
	}
}

// The alphabets are pinned rather than described. What a generated password may
// contain is a decision — no quotes, no backslashes, nothing a shell or a YAML
// file will argue about — and a decision that only exists as a string literal is
// one a refactor can change without anybody noticing.
func TestTheAlphabetsMatchTheBrowsers(t *testing.T) {
	tests := []struct {
		shape Shape
		want  string
	}{
		{
			shape: ShapeAlphanumeric,
			want:  "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789",
		},
		{
			shape: ShapePassword,
			want:  "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!#%*+-.:=?@^_~",
		},
	}

	for _, test := range tests {
		t.Run(string(test.shape), func(t *testing.T) {
			// Every character has to be reachable, and no character outside the set
			// may appear. Enough draws that a missing character is a failure rather
			// than a coincidence: 76 characters over 40,000 draws.
			seen := map[rune]bool{}
			for range 500 {
				value, err := Generate(test.shape, MaxLength)
				if err != nil {
					t.Fatalf("Generate() error = %v", err)
				}
				for _, r := range value {
					seen[r] = true
					if !strings.ContainsRune(test.want, r) {
						t.Fatalf("drew %q, which is not in the alphabet", r)
					}
				}
			}
			for _, r := range test.want {
				if !seen[r] {
					t.Errorf("%q was never drawn — is it reachable?", r)
				}
			}
		})
	}
}

// Rejection sampling, which is the reason fromAlphabet is not three lines.
//
// Checked as a distribution rather than by injecting a source, because crypto/rand
// is not injectable here and should not be made so to suit a test. A modulo fold
// over a 76-character alphabet would make the first 256%76 = 28 characters
// 4/3 as likely as the rest, and that gap is wide enough to see in a sample this
// size while ordinary variance is not.
func TestTheAlphabetIsSampledWithoutBias(t *testing.T) {
	const draws = 400
	alphabet := letters + digits + symbols

	counts := map[rune]int{}
	total := 0
	for range draws {
		value, err := Generate(ShapePassword, MaxLength)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		for _, r := range value {
			counts[r]++
			total++
		}
	}

	// The characters a modulo fold would favour, against the ones it would not.
	overweight := 256 % len(alphabet)
	var favoured, rest int
	for i, r := range alphabet {
		if i < overweight {
			favoured += counts[r]
		} else {
			rest += counts[r]
		}
	}

	favouredPer := float64(favoured) / float64(overweight)
	restPer := float64(rest) / float64(len(alphabet)-overweight)
	ratio := favouredPer / restPer

	// Biased sampling would put this near 1.33. Uniform sampling puts it at 1.00,
	// and the window is wide enough that this does not flake.
	if ratio < 0.9 || ratio > 1.1 {
		t.Errorf("the first %d characters came up %.2fx as often as the rest — "+
			"that is what a modulo fold looks like (total %d)", overweight, ratio, total)
	}
}

func TestGenerateRefusesWhatItWillNotMake(t *testing.T) {
	tests := []struct {
		name   string
		shape  Shape
		length int
	}{
		{name: "an unknown shape", shape: "uuid", length: DefaultLength},
		{name: "an empty shape", shape: "", length: DefaultLength},
		{name: "too short", shape: ShapePassword, length: MinLength - 1},
		{name: "too long", shape: ShapePassword, length: MaxLength + 1},
		{name: "zero", shape: ShapePassword, length: 0},
		{name: "negative", shape: ShapePassword, length: -8},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := Generate(test.shape, test.length)
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("Generate() error = %v, want %v", err, ErrInvalidRequest)
			}
			// A refusal that also returned something would be the worst outcome:
			// a caller ignoring the error installs a credential nobody chose.
			if value != "" {
				t.Errorf("a refused request returned %q", value)
			}
		})
	}
}

func TestGenerateDoesNotRepeatItself(t *testing.T) {
	seen := map[string]bool{}
	for range 200 {
		value, err := Generate(ShapePassword, DefaultLength)
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if seen[value] {
			t.Fatalf("generated %q twice", value)
		}
		seen[value] = true
	}
}
