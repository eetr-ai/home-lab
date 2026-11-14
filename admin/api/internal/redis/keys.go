package redis

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"unicode/utf8"
)

// The bounds on reading one key's value.
//
// maxItems caps how many items of a collection are shown, and maxValueBytes
// caps how many bytes of it are read at all. The second is the one that
// protects the API: LRANGE and its kind limit a count, not a size, so 100 items
// of a megabyte each is 100 MB through one call. The read is sized before it is
// made (see itemBudget), not trimmed after.
//
// maxElementBytes caps each single value shown — one list item, one hash value —
// so one huge element cannot fill the response the others were budgeted out of.
const (
	maxItems        = 100
	maxStringBytes  = 64 * 1024
	maxValueBytes   = 256 * 1024
	maxElementBytes = 4 * 1024
)

// The types TYPE answers with that this panel reads.
const (
	typeString = "string"
	typeList   = "list"
	typeSet    = "set"
	typeHash   = "hash"
	typeZSet   = "zset"
	typeStream = "stream"
)

// KeyStats is what a key is, without its value.
type KeyStats struct {
	Type      string
	TTLMillis int64
	// MemoryBytes is Redis's own estimate of what the key costs, zero when it
	// would not say.
	MemoryBytes int64
	// Length is bytes for a string and items for everything else.
	Length int64
}

// RawPair is one name/value pair as it came off the wire.
type RawPair struct{ Name, Value string }

// RawScored is one sorted-set member as it came off the wire.
type RawScored struct {
	Member string
	Score  float64
}

// RawEntry is one stream entry as it came off the wire.
type RawEntry struct {
	ID     string
	Fields []RawPair
}

// RawKey is a key's value as the repository read it, before this service has
// bounded or encoded it. Only the field matching Type is populated.
type RawKey struct {
	Type string

	String  string
	Items   []string
	Pairs   []RawPair
	Scored  []RawScored
	Entries []RawEntry
}

// readableTypes are the types this panel knows how to show. Anything else —
// Redis 8 ships JSON, time series and probabilistic structures in the box — is
// reported as existing, with its size, and not read.
var readableTypes = map[string]bool{
	typeString: true, typeList: true, typeSet: true, typeHash: true, typeZSet: true, typeStream: true,
}

// ReadKey opens one key, bounded in count and in bytes.
func (s *Service) ReadKey(ctx context.Context, key string) (KeyDetail, error) {
	if err := validateKey(key); err != nil {
		return KeyDetail{}, err
	}

	stats, err := s.repo.Inspect(ctx, key)
	if err != nil {
		return KeyDetail{}, err
	}
	if stats.Type == typeNone || stats.TTLMillis == ttlMissing {
		return KeyDetail{}, fmt.Errorf("%w: there is no key %q", ErrNotFound, key)
	}

	ret := KeyDetail{
		Key:         encode(key),
		Type:        stats.Type,
		TTLMillis:   stats.TTLMillis,
		MemoryBytes: stats.MemoryBytes,
		Length:      stats.Length,
		Supported:   readableTypes[stats.Type],
	}
	if !ret.Supported {
		return ret, nil
	}

	raw, err := s.repo.ReadValue(ctx, key, stats.Type, itemBudget(stats.MemoryBytes, stats.Length), maxStringBytes)
	if err != nil {
		return KeyDetail{}, err
	}
	fillValue(&ret, raw, stats.Length)
	return ret, nil
}

// itemBudget is how many items of a collection to read: maxItems, or fewer
// when the items are big enough that maxItems of them would pass maxValueBytes.
//
// It sizes the read from Redis's memory estimate for the whole key divided by
// its length. That is an average over a sample, so a key with one giant item
// among small ones can still go over; maxElementBytes then bounds what is shown,
// and the 128 MB ceiling the platform runs Redis with bounds the worst case.
// With no estimate (MEMORY USAGE refused or unknown) it falls back to the count
// alone.
//
// One more than is shown, so a key holding exactly the shown count is not
// reported as truncated and one holding one more is.
func itemBudget(memoryBytes, length int64) int {
	ret := maxItems + 1
	if memoryBytes <= 0 || length <= 0 {
		return ret
	}
	perItem := max(memoryBytes/length, 1)
	if fits := maxValueBytes / perItem; fits < int64(ret) {
		ret = int(max(fits, 1))
	}
	return ret
}

// fillValue puts the bounded, encoded value of a raw key on its detail.
func fillValue(ret *KeyDetail, raw RawKey, length int64) {
	if raw.Type == typeString {
		value, _ := clip(raw.String, maxStringBytes)
		// GETRANGE stops at a byte, not a character, so a long UTF-8 string read
		// in part can end in half a rune — and then the whole preview fails the
		// UTF-8 check and is shown as base64. Only when the read was cut short.
		if int64(len(value)) < length {
			value = trimPartialRune(value)
		}
		text := encode(value)
		ret.String = &text
		ret.Truncated = int64(len(value)) < length
		return
	}

	var c clipper
	shown := 0
	switch raw.Type {
	case typeList, typeSet:
		items := bounded(raw.Items)
		ret.Items = make([]Text, len(items))
		for i, item := range items {
			ret.Items[i] = c.text(item)
		}
		shown = len(items)
	case typeHash:
		pairs := bounded(raw.Pairs)
		ret.Fields = c.fields(pairs)
		shown = len(pairs)
	case typeZSet:
		members := bounded(raw.Scored)
		ret.Members = make([]ScoredMember, len(members))
		for i, member := range members {
			ret.Members[i] = ScoredMember{Member: c.text(member.Member), Score: formatScore(member.Score)}
		}
		shown = len(members)
	case typeStream:
		entries := bounded(raw.Entries)
		ret.Entries = make([]StreamEntry, len(entries))
		for i, entry := range entries {
			ret.Entries[i] = StreamEntry{ID: entry.ID, Fields: c.fields(entry.Fields)}
		}
		shown = len(entries)
	}
	ret.Truncated = c.clipped || int64(shown) < length
}

// clipper caps each value it encodes at maxElementBytes and remembers whether
// it had to, so a key can say it is truncated even when every item is shown.
type clipper struct{ clipped bool }

func (c *clipper) text(value string) Text {
	shown, cut := clip(value, maxElementBytes)
	c.clipped = c.clipped || cut
	return encode(shown)
}

func (c *clipper) fields(pairs []RawPair) []Field {
	ret := make([]Field, len(pairs))
	for i, pair := range pairs {
		ret[i] = Field{Name: c.text(pair.Name), Value: c.text(pair.Value)}
	}
	return ret
}

// formatScore writes a sorted-set score as Redis does.
//
// A string rather than a number because a score can be infinite — ZADD takes
// +inf and -inf — and JSON has no way to write that. As a float64 field, one
// such member made the encoder fail after the 200 was already sent, and the
// panel received an empty body for a key it could not then open.
func formatScore(score float64) string {
	switch {
	case math.IsInf(score, 1):
		return "inf"
	case math.IsInf(score, -1):
		return "-inf"
	}
	return strconv.FormatFloat(score, 'g', -1, 64)
}

// clip cuts a value to at most limit bytes, and reports whether it did.
//
// It backs off to the start of a character when the value is text, so a long
// UTF-8 value cut mid-character is still shown as text rather than turning into
// base64 because its last byte is half a rune.
func clip(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	ret := value[:limit]
	if utf8.ValidString(value) {
		for len(ret) > 0 && !utf8.RuneStart(value[len(ret)]) {
			ret = ret[:len(ret)-1]
		}
	}
	return ret, true
}

// trimPartialRune drops an incomplete UTF-8 character from the end of a value
// that is otherwise text.
//
// It touches nothing else: a value that is already valid comes back as it is,
// and so does one whose invalid bytes are not just a cut-off tail — that is
// binary, and base64 is the honest way to show it.
func trimPartialRune(value string) string {
	if utf8.ValidString(value) {
		return value
	}
	for cut := 1; cut < utf8.UTFMax && cut < len(value); cut++ {
		head, tail := value[:len(value)-cut], value[len(value)-cut:]
		if utf8.RuneStart(tail[0]) && !utf8.FullRuneInString(tail) && utf8.ValidString(head) {
			return head
		}
	}
	return value
}

// bounded keeps the first maxItems of anything.
func bounded[T any](items []T) []T {
	if len(items) > maxItems {
		return items[:maxItems]
	}
	return items
}

// encode wraps bytes from Redis for a JSON response.
func encode(value string) Text {
	if utf8.ValidString(value) {
		return Text{Value: value, Encoding: encodingUTF8}
	}
	return Text{Value: base64.StdEncoding.EncodeToString([]byte(value)), Encoding: encodingBase64}
}
