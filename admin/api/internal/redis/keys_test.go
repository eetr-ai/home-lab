package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

// items returns n distinct strings, for building collections at and just past
// the bound.
func items(n int) []string {
	ret := make([]string, n)
	for i := range ret {
		ret[i] = fmt.Sprintf("item-%d", i)
	}
	return ret
}

func TestReadKey(t *testing.T) {
	tests := []struct {
		name          string
		stats         KeyStats
		raw           RawKey
		wantShown     int
		wantTruncated bool
		wantSupported bool
		wantRead      bool
		wantErr       error
	}{
		{
			name:          "a list holding exactly the bound is not truncated",
			stats:         KeyStats{Type: "list", TTLMillis: ttlNone, Length: maxItems},
			raw:           RawKey{Type: "list", Items: items(maxItems)},
			wantShown:     maxItems,
			wantSupported: true,
			wantRead:      true,
		},
		{
			// The repository is asked for one more than we show, and that extra item
			// is the whole signal: it is what makes this "truncated" without asking
			// the server a second question.
			name:          "a list one past the bound shows the bound and says so",
			stats:         KeyStats{Type: "list", TTLMillis: ttlNone, Length: 5000},
			raw:           RawKey{Type: "list", Items: items(maxItems + 1)},
			wantShown:     maxItems,
			wantTruncated: true,
			wantSupported: true,
			wantRead:      true,
		},
		{
			name:          "a string longer than the byte bound is truncated",
			stats:         KeyStats{Type: "string", TTLMillis: ttlNone, Length: maxStringBytes + 10},
			raw:           RawKey{Type: "string", String: "abc"},
			wantTruncated: true,
			wantSupported: true,
			wantRead:      true,
		},
		{
			name:          "a short string is whole",
			stats:         KeyStats{Type: "string", TTLMillis: ttlNone, Length: 3},
			raw:           RawKey{Type: "string", String: "abc"},
			wantSupported: true,
			wantRead:      true,
		},
		{
			// Nothing to read with, so nothing is read: the module's bytes would
			// only be guessed at.
			name:          "a module's type is reported but not read",
			stats:         KeyStats{Type: "ReJSON-RL", TTLMillis: ttlNone, Length: 1},
			wantSupported: false,
		},
		{name: "a missing key is not found", stats: KeyStats{Type: typeNone, TTLMillis: ttlMissing}, wantErr: ErrNotFound},
		{
			name:    "a key that expired mid-read is not found",
			stats:   KeyStats{Type: "string", TTLMillis: ttlMissing},
			wantErr: ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := fakeRepo{stats: tt.stats, raw: tt.raw}
			detail, err := NewService(&repo).ReadKey(context.Background(), "k")

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ReadKey() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadKey() unexpected error: %v", err)
			}
			if read := repo.gotItems != 0; read != tt.wantRead {
				t.Errorf("value read = %v, want %v", read, tt.wantRead)
			}
			if len(detail.Items) != tt.wantShown {
				t.Errorf("ReadKey() showed %d items, want %d", len(detail.Items), tt.wantShown)
			}
			if detail.Truncated != tt.wantTruncated {
				t.Errorf("ReadKey() truncated = %v, want %v", detail.Truncated, tt.wantTruncated)
			}
			if detail.Supported != tt.wantSupported {
				t.Errorf("ReadKey() supported = %v, want %v", detail.Supported, tt.wantSupported)
			}
		})
	}
}

// The read is sized before it is made. These are the numbers that decide how
// much of a collection crosses the wire, so they are worth pinning.
func TestItemBudget(t *testing.T) {
	tests := []struct {
		name   string
		memory int64
		length int64
		want   int
	}{
		{name: "small items read the full count", memory: 10_000, length: 1000, want: maxItems + 1},
		{
			// 1 MiB items: 100 of them would be 100 MiB. maxValueBytes allows none
			// whole, so one is read — enough to show what the key holds.
			name: "huge items read one", memory: 100 << 20, length: 100, want: 1,
		},
		{
			// 16 KiB items: 256 KiB fits 16 of them.
			name: "large items read what fits", memory: 1000 * 16 << 10, length: 1000, want: 16,
		},
		{name: "no memory estimate falls back to the count", memory: 0, length: 1000, want: maxItems + 1},
		{name: "an empty key reads the count", memory: 100, length: 0, want: maxItems + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := itemBudget(tt.memory, tt.length); got != tt.want {
				t.Errorf("itemBudget(%d, %d) = %d, want %d", tt.memory, tt.length, got, tt.want)
			}
		})
	}
}

func TestReadKeyAsksForTheBudgetedCount(t *testing.T) {
	repo := fakeRepo{
		stats: KeyStats{Type: "list", TTLMillis: ttlNone, MemoryBytes: 100 << 20, Length: 100},
		raw:   RawKey{Type: "list", Items: items(1)},
	}
	detail, err := NewService(&repo).ReadKey(context.Background(), "k")
	if err != nil {
		t.Fatalf("ReadKey() unexpected error: %v", err)
	}
	if repo.gotItems != 1 {
		t.Errorf("asked the repository for %d items, want 1 for 1 MiB items", repo.gotItems)
	}
	if !detail.Truncated {
		t.Error("one item of a hundred must be reported as truncated")
	}
}

func TestOneHugeElementIsClipped(t *testing.T) {
	repo := fakeRepo{
		stats: KeyStats{Type: "hash", TTLMillis: ttlNone, Length: 1},
		raw:   RawKey{Type: "hash", Pairs: []RawPair{{Name: "f", Value: strings.Repeat("x", 1<<20)}}},
	}
	detail, err := NewService(&repo).ReadKey(context.Background(), "k")
	if err != nil {
		t.Fatalf("ReadKey() unexpected error: %v", err)
	}
	if got := len(detail.Fields[0].Value.Value); got != maxElementBytes {
		t.Errorf("value shown is %d bytes, want %d", got, maxElementBytes)
	}
	if !detail.Truncated {
		t.Error("a clipped value must be reported as truncated, even with every item shown")
	}
}

func TestClipStaysOnACharacterBoundary(t *testing.T) {
	// Each ñ is two bytes; cutting at an odd limit would split one and turn the
	// whole value into base64 for the sake of its last byte.
	got, cut := clip(strings.Repeat("ñ", 10), 5)
	if !cut || got != "ññ" || !utf8.ValidString(got) {
		t.Errorf("clip() = %q, %v; want %q, true", got, cut, "ññ")
	}
}

// The regression CodeRabbit described, as the read actually returns it: 65,535
// ASCII bytes and then the first byte of a two-byte ñ, because GETRANGE stopped
// at maxStringBytes.
func TestAStringReadCutMidCharacterIsStillText(t *testing.T) {
	read := strings.Repeat("a", maxStringBytes-1) + "\xc3"
	repo := fakeRepo{
		stats: KeyStats{Type: "string", TTLMillis: ttlNone, Length: maxStringBytes + 1},
		raw:   RawKey{Type: "string", String: read},
	}
	detail, err := NewService(&repo).ReadKey(context.Background(), "k")
	if err != nil {
		t.Fatalf("ReadKey() unexpected error: %v", err)
	}
	if detail.String.Encoding != encodingUTF8 || len(detail.String.Value) != maxStringBytes-1 {
		t.Errorf("preview is %s with %d bytes; want utf8 with the half character dropped",
			detail.String.Encoding, len(detail.String.Value))
	}
}

func TestTrimPartialRuneLeavesBinaryAlone(t *testing.T) {
	// Invalid bytes in the middle are not a cut-off tail: that is binary.
	binary := "\xff\xfeabc"
	if got := trimPartialRune(binary); got != binary {
		t.Errorf("trimPartialRune(%q) = %q, want it unchanged", binary, got)
	}
	if got := trimPartialRune("ok"); got != "ok" {
		t.Errorf("a valid value must come back as it is, got %q", got)
	}
}

func TestScoresSurviveJSON(t *testing.T) {
	// The bug this guards: a float64 score of +Inf made encoding/json fail after
	// the 200 was written, and the panel got an empty body.
	members := []ScoredMember{
		{Score: formatScore(math.Inf(1))},
		{Score: formatScore(math.Inf(-1))},
		{Score: formatScore(1.5)},
		{Score: formatScore(100)},
	}
	body, err := json.Marshal(members)
	if err != nil {
		t.Fatalf("scores did not encode: %v", err)
	}
	for _, want := range []string{`"inf"`, `"-inf"`, `"1.5"`, `"100"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("encoded scores %s do not contain %s", body, want)
		}
	}
}
