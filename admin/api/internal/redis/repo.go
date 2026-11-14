package redis

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	goredis "github.com/redis/go-redis/v9"
)

// maxScanRounds bounds the HSCAN and SSCAN loops that fill an opened hash or set.
// Those commands return roughly `count` elements per call but may return fewer,
// and a loop that ran until it had enough could run for a long time on a large,
// sparse collection. A few rounds and whatever they found is a fair answer to "show
// me the first hundred".
const maxScanRounds = 8

// Repository is the slice's connection to Redis. It translates between the
// driver and the slice's own types, and holds no rules.
type Repository struct {
	client *goredis.Client
}

// NewRepository builds a client for a redis:// URL and a password kept apart
// from it.
//
// The password is its own argument rather than userinfo in the URL because it
// arrives from its own Secret, and a URL with a credential in it is a credential
// that ends up in a log line the first time anything prints the configuration.
//
// Building it does not connect: go-redis dials on the first command, so an
// unreachable server costs a failed request rather than an API that will not
// start — the same choice the database slices make.
func NewRepository(url, password string) (*Repository, error) {
	options, err := goredis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse the Redis URL: %w", err)
	}
	options.Password = password
	return &Repository{client: goredis.NewClient(options)}, nil
}

// Close releases the connection pool.
func (r *Repository) Close() error {
	if err := r.client.Close(); err != nil {
		return fmt.Errorf("close the Redis client: %w", err)
	}
	return nil
}

// Info returns the sections the overview reads.
func (r *Repository) Info(ctx context.Context) (string, error) {
	info, err := r.client.Info(ctx, "server", "clients", "memory", "stats", "errorstats", "keyspace").Result()
	if err != nil {
		return "", fmt.Errorf("read INFO: %w", err)
	}
	return info, nil
}

// Scan runs one SCAN step.
func (r *Repository) Scan(ctx context.Context, cursor uint64, match string, count int64) ([]string, uint64, error) {
	keys, next, err := r.client.Scan(ctx, cursor, match, count).Result()
	if err != nil {
		return nil, 0, fmt.Errorf("scan keys: %w", err)
	}
	return keys, next, nil
}

// Describe reads the type and TTL of each key in one round trip.
//
// TYPE and PTTL go through Do rather than the typed helpers so their raw replies
// come back: go-redis turns PTTL's -1 and -2 into durations, which are easy to
// misread as "one nanosecond left".
func (r *Repository) Describe(ctx context.Context, keys []string) ([]KeyMeta, error) {
	pipe := r.client.Pipeline()
	types := make([]*goredis.Cmd, len(keys))
	ttls := make([]*goredis.Cmd, len(keys))
	for i, key := range keys {
		types[i] = pipe.Do(ctx, "TYPE", key)
		ttls[i] = pipe.Do(ctx, "PTTL", key)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("describe keys: %w", err)
	}

	ret := make([]KeyMeta, len(keys))
	for i := range keys {
		kind, err := types[i].Text()
		if err != nil {
			return nil, fmt.Errorf("read a key's type: %w", err)
		}
		ttl, err := ttls[i].Int64()
		if err != nil {
			return nil, fmt.Errorf("read a key's TTL: %w", err)
		}
		ret[i] = KeyMeta{Type: kind, TTLMillis: ttl}
	}
	return ret, nil
}

// ReadKey reads a key's metadata and the first part of its value.
func (r *Repository) ReadKey(ctx context.Context, key string, items int, bytes int64) (RawKey, error) {
	metas, err := r.Describe(ctx, []string{key})
	if err != nil {
		return RawKey{}, err
	}
	ret := RawKey{Type: metas[0].Type, TTLMillis: metas[0].TTLMillis}
	if ret.Type == typeNone {
		return ret, nil
	}

	// A nice-to-have: an error here leaves the figure at zero rather than failing
	// a read whose point is the value.
	if memory, err := r.client.MemoryUsage(ctx, key).Result(); err == nil {
		ret.MemoryBytes = memory
	}

	switch ret.Type {
	case "string":
		err = r.readString(ctx, key, bytes, &ret)
	case "list":
		err = r.readList(ctx, key, items, &ret)
	case "set":
		err = r.readSet(ctx, key, items, &ret)
	case "hash":
		err = r.readHash(ctx, key, items, &ret)
	case "zset":
		err = r.readSortedSet(ctx, key, items, &ret)
	case "stream":
		err = r.readStream(ctx, key, items, &ret)
	}
	// A key that went away between TYPE and the read answers redis.Nil or an
	// empty reply; either way the next request reports it gone, and this one
	// shows what it got.
	if err != nil && !errors.Is(err, goredis.Nil) {
		return RawKey{}, fmt.Errorf("read %s key: %w", ret.Type, err)
	}
	return ret, nil
}

//nolint:wrapcheck // ReadKey wraps every reader's error once, with the type in it.
func (r *Repository) readString(ctx context.Context, key string, bytes int64, ret *RawKey) error {
	length, err := r.client.StrLen(ctx, key).Result()
	if err != nil {
		return err
	}
	ret.Length = length
	ret.String, err = r.client.GetRange(ctx, key, 0, bytes-1).Result()
	return err
}

//nolint:wrapcheck // ReadKey wraps every reader's error once, with the type in it.
func (r *Repository) readList(ctx context.Context, key string, items int, ret *RawKey) error {
	length, err := r.client.LLen(ctx, key).Result()
	if err != nil {
		return err
	}
	ret.Length = length
	ret.Items, err = r.client.LRange(ctx, key, 0, int64(items)-1).Result()
	return err
}

//nolint:wrapcheck // ReadKey wraps every reader's error once, with the type in it.
func (r *Repository) readSet(ctx context.Context, key string, items int, ret *RawKey) error {
	length, err := r.client.SCard(ctx, key).Result()
	if err != nil {
		return err
	}
	ret.Length = length

	var cursor uint64
	for range maxScanRounds {
		var page []string
		page, cursor, err = r.client.SScan(ctx, key, cursor, "", int64(items)).Result()
		if err != nil {
			return err
		}
		ret.Items = append(ret.Items, page...)
		if cursor == 0 || len(ret.Items) >= items {
			break
		}
	}
	return nil
}

//nolint:wrapcheck // ReadKey wraps every reader's error once, with the type in it.
func (r *Repository) readHash(ctx context.Context, key string, items int, ret *RawKey) error {
	length, err := r.client.HLen(ctx, key).Result()
	if err != nil {
		return err
	}
	ret.Length = length

	var cursor uint64
	for range maxScanRounds {
		var page []string
		page, cursor, err = r.client.HScan(ctx, key, cursor, "", int64(items)).Result()
		if err != nil {
			return err
		}
		// HSCAN answers a flat list: field, value, field, value.
		for i := 0; i+1 < len(page); i += 2 {
			ret.Pairs = append(ret.Pairs, RawPair{Name: page[i], Value: page[i+1]})
		}
		if cursor == 0 || len(ret.Pairs) >= items {
			break
		}
	}
	return nil
}

//nolint:wrapcheck // ReadKey wraps every reader's error once, with the type in it.
func (r *Repository) readSortedSet(ctx context.Context, key string, items int, ret *RawKey) error {
	length, err := r.client.ZCard(ctx, key).Result()
	if err != nil {
		return err
	}
	ret.Length = length

	members, err := r.client.ZRangeWithScores(ctx, key, 0, int64(items)-1).Result()
	if err != nil {
		return err
	}
	for _, member := range members {
		ret.Scored = append(ret.Scored, RawScored{Member: fmt.Sprint(member.Member), Score: member.Score})
	}
	return nil
}

//nolint:wrapcheck // ReadKey wraps every reader's error once, with the type in it.
func (r *Repository) readStream(ctx context.Context, key string, items int, ret *RawKey) error {
	length, err := r.client.XLen(ctx, key).Result()
	if err != nil {
		return err
	}
	ret.Length = length

	entries, err := r.client.XRangeN(ctx, key, "-", "+", int64(items)).Result()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		// go-redis hands an entry's fields back as a map, which has already lost
		// the order they were written in. Sorting at least makes the same entry
		// read the same way twice.
		raw := RawEntry{ID: entry.ID}
		for _, name := range slices.Sorted(maps.Keys(entry.Values)) {
			raw.Fields = append(raw.Fields, RawPair{Name: name, Value: fmt.Sprint(entry.Values[name])})
		}
		ret.Entries = append(ret.Entries, raw)
	}
	return nil
}

// Unlink removes a key without blocking on freeing its memory.
func (r *Repository) Unlink(ctx context.Context, key string) (int64, error) {
	removed, err := r.client.Unlink(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("unlink key: %w", err)
	}
	return removed, nil
}
