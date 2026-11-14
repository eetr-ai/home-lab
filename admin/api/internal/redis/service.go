package redis

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"unicode/utf8"
)

// The bounds on what one request may ask of the server.
//
// This Redis holds live locks for the panel's own sign-in, and it is single
// threaded: a command that takes a second is a second in which every lock waits.
// So nothing here asks for an unbounded amount of anything. A scan page is a hint
// of at most maxScanCount, and an opened key returns the first maxItems items and
// the first maxStringBytes bytes, never the whole thing.
const (
	defaultScanCount = 100
	maxScanCount     = 1000
	maxPatternLength = 512
	maxKeyLength     = 4096
	maxItems         = 100
	maxStringBytes   = 64 * 1024
)

// The values PTTL answers with when there is no TTL to report.
const (
	ttlNone    = -1
	ttlMissing = -2
)

// typeNone is what TYPE answers for a key that does not exist.
const typeNone = "none"

// KeyMeta is what a listing needs to know about a key.
type KeyMeta struct {
	Type      string
	TTLMillis int64
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

// RawKey is a key read back by the repository, before this service has bounded
// it or decided how to encode it. Only the field matching Type is populated.
type RawKey struct {
	Type        string
	TTLMillis   int64
	MemoryBytes int64
	Length      int64

	String  string
	Items   []string
	Pairs   []RawPair
	Scored  []RawScored
	Entries []RawEntry
}

// repository is the persistence this service needs. Declared here, where it is
// consumed, so the service can be tested without a Redis server.
type repository interface {
	Info(ctx context.Context) (string, error)
	Scan(ctx context.Context, cursor uint64, match string, count int64) ([]string, uint64, error)
	Describe(ctx context.Context, keys []string) ([]KeyMeta, error)
	// ReadKey reads at most `items` items and `bytes` bytes of a key. A key that
	// does not exist comes back with Type "none".
	ReadKey(ctx context.Context, key string, items int, bytes int64) (RawKey, error)
	// Unlink removes a key and reports how many it removed.
	Unlink(ctx context.Context, key string) (int64, error)
}

// Service reads and tends the platform's shared Redis.
//
// The panel connects as the server's default user, which can do anything — so,
// as with the databases, what this service declines to do is the boundary, not
// what the account could. There is no flush here on purpose: the server holds the
// token-refresh locks that keep operators signed in, and emptying it signs them
// out. One key at a time is the most this panel will remove.
type Service struct {
	repo repository
}

// NewService builds the service.
func NewService(repo repository) *Service {
	return &Service{repo: repo}
}

// Overview returns what the server says about itself.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	info, err := s.repo.Info(ctx)
	if err != nil {
		return Overview{}, err
	}
	return parseInfo(info), nil
}

// ScanKeys returns one page of keys matching a glob pattern.
//
// The cursor is whatever the previous page returned, and empty to start. A count
// of zero means the default; anything above the ceiling is brought down to it
// rather than refused, because a count is only ever a hint to SCAN anyway.
func (s *Service) ScanKeys(ctx context.Context, pattern, cursor string, count int) (KeyPage, error) {
	pattern, start, count, err := scanArguments(pattern, cursor, count)
	if err != nil {
		return KeyPage{}, err
	}

	keys, next, err := s.repo.Scan(ctx, start, pattern, int64(count))
	if err != nil {
		return KeyPage{}, err
	}

	ret := KeyPage{Keys: []KeySummary{}}
	// SCAN answers 0 when the iteration is finished. Handing "0" back would read
	// as "start again", which is exactly the loop a pager must not fall into.
	if next != 0 {
		ret.NextCursor = strconv.FormatUint(next, 10)
	}
	if len(keys) == 0 {
		return ret, nil
	}

	metas, err := s.repo.Describe(ctx, keys)
	if err != nil {
		return KeyPage{}, err
	}
	for i, key := range keys {
		// A key can expire or be deleted between the scan and the describe. It is
		// gone, so it is not on the page.
		if metas[i].Type == typeNone || metas[i].TTLMillis == ttlMissing {
			continue
		}
		ret.Keys = append(ret.Keys, KeySummary{Key: encode(key), Type: metas[i].Type, TTLMillis: metas[i].TTLMillis})
	}
	return ret, nil
}

// ReadKey opens one key, bounded.
func (s *Service) ReadKey(ctx context.Context, key string) (KeyDetail, error) {
	if err := validateKey(key); err != nil {
		return KeyDetail{}, err
	}

	// One more item than we show, so that a key holding exactly maxItems is not
	// reported as truncated and one holding maxItems+1 is.
	raw, err := s.repo.ReadKey(ctx, key, maxItems+1, maxStringBytes)
	if err != nil {
		return KeyDetail{}, err
	}
	if raw.Type == typeNone || raw.TTLMillis == ttlMissing {
		return KeyDetail{}, fmt.Errorf("%w: there is no key %q", ErrNotFound, key)
	}

	ret := KeyDetail{
		Key:         encode(key),
		Type:        raw.Type,
		TTLMillis:   raw.TTLMillis,
		MemoryBytes: raw.MemoryBytes,
		Length:      raw.Length,
		Supported:   true,
	}
	fillValue(&ret, raw)
	return ret, nil
}

// fillValue puts the bounded, encoded value of a raw key on its detail.
func fillValue(ret *KeyDetail, raw RawKey) {
	switch raw.Type {
	case "string":
		value := encode(raw.String)
		ret.String = &value
		ret.Truncated = int64(len(raw.String)) < raw.Length
	case "list", "set":
		items := bounded(raw.Items)
		ret.Items = make([]Text, len(items))
		for i, item := range items {
			ret.Items[i] = encode(item)
		}
		ret.Truncated = int64(len(items)) < raw.Length
	case "hash":
		pairs := bounded(raw.Pairs)
		ret.Fields = encodePairs(pairs)
		ret.Truncated = int64(len(pairs)) < raw.Length
	case "zset":
		members := bounded(raw.Scored)
		ret.Members = make([]ScoredMember, len(members))
		for i, member := range members {
			ret.Members[i] = ScoredMember{Member: encode(member.Member), Score: member.Score}
		}
		ret.Truncated = int64(len(members)) < raw.Length
	case "stream":
		entries := bounded(raw.Entries)
		ret.Entries = make([]StreamEntry, len(entries))
		for i, entry := range entries {
			ret.Entries[i] = StreamEntry{ID: entry.ID, Fields: encodePairs(entry.Fields)}
		}
		ret.Truncated = int64(len(entries)) < raw.Length
	default:
		// A module's type — Redis 8 ships JSON, time series and probabilistic
		// structures in the box. We show that the key exists and what it is, and
		// say plainly that we cannot read it rather than guess at its encoding.
		ret.Supported = false
	}
}

// DeleteKey removes one key.
//
// UNLINK rather than DEL: it frees the value on a background thread, so removing
// a large key does not stall the server — and every lock in it — while it does.
func (s *Service) DeleteKey(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	removed, err := s.repo.Unlink(ctx, key)
	if err != nil {
		return err
	}
	if removed == 0 {
		return fmt.Errorf("%w: there is no key %q", ErrNotFound, key)
	}
	return nil
}

// scanArguments checks and defaults what a scan was asked for.
func scanArguments(pattern, cursor string, count int) (string, uint64, int, error) {
	if len(pattern) > maxPatternLength {
		return "", 0, 0, fmt.Errorf("%w: the pattern is longer than %d bytes", ErrInvalidRequest, maxPatternLength)
	}
	if pattern == "" {
		pattern = "*"
	}

	start, err := parseCursor(cursor)
	if err != nil {
		return "", 0, 0, err
	}

	switch {
	case count < 0:
		return "", 0, 0, fmt.Errorf("%w: count cannot be negative", ErrInvalidRequest)
	case count == 0:
		count = defaultScanCount
	case count > maxScanCount:
		count = maxScanCount
	}
	return pattern, start, count, nil
}

// parseCursor reads a cursor this service handed out, or the empty one.
func parseCursor(cursor string) (uint64, error) {
	if cursor == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(cursor, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q is not a cursor this panel handed out", ErrInvalidRequest, cursor)
	}
	return n, nil
}

// validateKey refuses the key names no request could mean.
func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("%w: a key name is required", ErrInvalidRequest)
	}
	if len(key) > maxKeyLength {
		return fmt.Errorf("%w: the key name is longer than %d bytes", ErrInvalidRequest, maxKeyLength)
	}
	return nil
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

// encodePairs wraps every name and value in a list of pairs.
func encodePairs(pairs []RawPair) []Field {
	ret := make([]Field, len(pairs))
	for i, pair := range pairs {
		ret[i] = Field{Name: encode(pair.Name), Value: encode(pair.Value)}
	}
	return ret
}
