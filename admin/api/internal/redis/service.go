package redis

import (
	"context"
	"fmt"
	"strconv"
)

// The bounds on what one request may ask of the server.
//
// This Redis holds live locks for the panel's own sign-in, and it is single
// threaded: a command that takes a second is a second in which every lock waits.
// So nothing here asks for an unbounded amount of anything. A scan page is a hint
// of at most maxScanCount, and an opened key is bounded in keys.go.
const (
	defaultScanCount = 100
	maxScanCount     = 1000
	maxPatternLength = 512
	maxKeyLength     = 4096
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

// repository is the persistence this service needs. Declared here, where it is
// consumed, so the service can be tested without a Redis server.
type repository interface {
	Info(ctx context.Context) (string, error)
	Scan(ctx context.Context, cursor uint64, match string, count int64) ([]string, uint64, error)
	Describe(ctx context.Context, keys []string) ([]KeyMeta, error)
	// Inspect reads a key's type, TTL, memory and length, but not its value. A
	// key that does not exist comes back with Type "none".
	Inspect(ctx context.Context, key string) (KeyStats, error)
	// ReadValue reads at most `items` items, or `bytes` bytes of a string.
	ReadValue(ctx context.Context, key, kind string, items int, bytes int64) (RawKey, error)
	// Unlink removes a key and reports how many it removed.
	Unlink(ctx context.Context, key string) (int64, error)

	userRepository
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
