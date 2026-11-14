package redis

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

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

// lengthCommands answers how many items each type holds, in the command that
// counts them without reading them: bytes for a string, items for the rest.
var lengthCommands = map[string]string{
	typeString: "STRLEN",
	typeList:   "LLEN",
	typeSet:    "SCARD",
	typeHash:   "HLEN",
	typeZSet:   "ZCARD",
	typeStream: "XLEN",
}

// Inspect reads what a key is without reading its value: type, TTL, the memory
// Redis attributes to it, and its length. A key that does not exist comes back
// with Type "none".
func (r *Repository) Inspect(ctx context.Context, key string) (KeyStats, error) {
	metas, err := r.Describe(ctx, []string{key})
	if err != nil {
		return KeyStats{}, err
	}
	ret := KeyStats{Type: metas[0].Type, TTLMillis: metas[0].TTLMillis}
	if ret.Type == typeNone {
		return ret, nil
	}

	// A nice-to-have for the panel, and the size estimate ReadValue's caller
	// budgets with. An error leaves it at zero, which the caller treats as "no
	// estimate" rather than as "empty".
	if memory, err := r.client.MemoryUsage(ctx, key).Result(); err == nil {
		ret.MemoryBytes = memory
	}

	if command, ok := lengthCommands[ret.Type]; ok {
		length, err := r.client.Do(ctx, command, key).Int64()
		if err != nil && !errors.Is(err, goredis.Nil) {
			return KeyStats{}, fmt.Errorf("measure %s key: %w", ret.Type, err)
		}
		ret.Length = length
	}
	return ret, nil
}

// ReadValue reads at most `items` items of a key of the given type, or at most
// `bytes` bytes of a string. How many is the caller's decision; this only
// reads what it was told to.
func (r *Repository) ReadValue(ctx context.Context, key, kind string, items int, bytes int64) (RawKey, error) {
	ret := RawKey{Type: kind}
	var err error
	switch kind {
	case typeString:
		ret.String, err = r.client.GetRange(ctx, key, 0, bytes-1).Result()
	case typeList:
		ret.Items, err = r.client.LRange(ctx, key, 0, int64(items)-1).Result()
	case typeSet:
		ret.Items, err = r.scanSet(ctx, key, items)
	case typeHash:
		ret.Pairs, err = r.scanHash(ctx, key, items)
	case typeZSet:
		ret.Scored, err = r.readSortedSet(ctx, key, items)
	case typeStream:
		ret.Entries, err = r.readStream(ctx, key, items)
	}
	// A key that went away between Inspect and the read answers redis.Nil or an
	// empty reply; either way the next request reports it gone, and this one
	// shows what it got.
	if err != nil && !errors.Is(err, goredis.Nil) {
		return RawKey{}, fmt.Errorf("read %s key: %w", kind, err)
	}
	return ret, nil
}

//nolint:wrapcheck // ReadValue wraps every reader's error once, with the type in it.
func (r *Repository) scanSet(ctx context.Context, key string, items int) ([]string, error) {
	var ret []string
	var cursor uint64
	for range maxScanRounds {
		page, next, err := r.client.SScan(ctx, key, cursor, "", int64(items)).Result()
		if err != nil {
			return nil, err
		}
		ret = append(ret, page...)
		cursor = next
		if cursor == 0 || len(ret) >= items {
			break
		}
	}
	return ret, nil
}

//nolint:wrapcheck // ReadValue wraps every reader's error once, with the type in it.
func (r *Repository) scanHash(ctx context.Context, key string, items int) ([]RawPair, error) {
	var ret []RawPair
	var cursor uint64
	for range maxScanRounds {
		page, next, err := r.client.HScan(ctx, key, cursor, "", int64(items)).Result()
		if err != nil {
			return nil, err
		}
		// HSCAN answers a flat list: field, value, field, value.
		for i := 0; i+1 < len(page); i += 2 {
			ret = append(ret, RawPair{Name: page[i], Value: page[i+1]})
		}
		cursor = next
		if cursor == 0 || len(ret) >= items {
			break
		}
	}
	return ret, nil
}

//nolint:wrapcheck // ReadValue wraps every reader's error once, with the type in it.
func (r *Repository) readSortedSet(ctx context.Context, key string, items int) ([]RawScored, error) {
	members, err := r.client.ZRangeWithScores(ctx, key, 0, int64(items)-1).Result()
	if err != nil {
		return nil, err
	}
	ret := make([]RawScored, 0, len(members))
	for _, member := range members {
		ret = append(ret, RawScored{Member: fmt.Sprint(member.Member), Score: member.Score})
	}
	return ret, nil
}

//nolint:wrapcheck // ReadValue wraps every reader's error once, with the type in it.
func (r *Repository) readStream(ctx context.Context, key string, items int) ([]RawEntry, error) {
	// The raw reply rather than go-redis's XRangeN, which decodes each entry's
	// fields into a map. A stream entry may repeat a field name (XADD s * tag a
	// tag b stores both), and a map keeps one and silently drops the other. The
	// raw reply is the entry exactly as stored, in its own order.
	reply, err := r.client.Do(ctx, "XRANGE", key, "-", "+", "COUNT", items).Slice()
	if err != nil {
		return nil, err
	}
	return parseStreamReply(reply)
}

// parseStreamReply reads XRANGE's reply: a list of [id, [field, value, ...]].
func parseStreamReply(reply []any) ([]RawEntry, error) {
	ret := make([]RawEntry, 0, len(reply))
	for _, item := range reply {
		entry, ok := item.([]any)
		if !ok || len(entry) != 2 {
			return nil, fmt.Errorf("unexpected XRANGE entry %T", item)
		}
		id, ok := entry[0].(string)
		if !ok {
			return nil, fmt.Errorf("unexpected XRANGE id %T", entry[0])
		}
		values, ok := entry[1].([]any)
		if !ok {
			return nil, fmt.Errorf("unexpected XRANGE fields %T", entry[1])
		}
		raw := RawEntry{ID: id}
		for i := 0; i+1 < len(values); i += 2 {
			raw.Fields = append(raw.Fields, RawPair{Name: fmt.Sprint(values[i]), Value: fmt.Sprint(values[i+1])})
		}
		ret = append(ret, raw)
	}
	return ret, nil
}

// ACLFile reads the aclfile setting the server started with.
func (r *Repository) ACLFile(ctx context.Context) (string, error) {
	values, err := r.client.ConfigGet(ctx, "aclfile").Result()
	if err != nil {
		return "", fmt.Errorf("read the aclfile setting: %w", err)
	}
	return values["aclfile"], nil
}

// ACLList answers ACL LIST.
func (r *Repository) ACLList(ctx context.Context) ([]string, error) {
	lines, err := r.client.Do(ctx, "ACL", "LIST").StringSlice()
	if err != nil {
		return nil, fmt.Errorf("list ACL users: %w", err)
	}
	return lines, nil
}

// ACLSetUser runs ACL SETUSER with the rules as given.
func (r *Repository) ACLSetUser(ctx context.Context, name string, rules []string) error {
	command := []any{"ACL", "SETUSER", name}
	args := make([]any, 0, len(command)+len(rules))
	args = append(args, command...)
	for _, rule := range rules {
		args = append(args, rule)
	}
	if err := r.client.Do(ctx, args...).Err(); err != nil {
		return fmt.Errorf("set ACL user %q: %w", name, err)
	}
	return nil
}

// ACLDelUser removes a user and reports how many it removed.
func (r *Repository) ACLDelUser(ctx context.Context, name string) (int64, error) {
	removed, err := r.client.Do(ctx, "ACL", "DELUSER", name).Int64()
	if err != nil {
		return 0, fmt.Errorf("delete ACL user %q: %w", name, err)
	}
	return removed, nil
}

// ACLSave writes the users to the aclfile.
func (r *Repository) ACLSave(ctx context.Context) error {
	if err := r.client.Do(ctx, "ACL", "SAVE").Err(); err != nil {
		return fmt.Errorf("save the aclfile: %w", err)
	}
	return nil
}

// lockWait is how long Lock keeps asking for a held lock, and lockRetry how
// often. An ACL change takes milliseconds, so a lock still held after this long
// belongs to something stuck, and the caller is better told than kept waiting.
const (
	lockWait  = 3 * time.Second
	lockRetry = 100 * time.Millisecond
	// lockTokenBytes of randomness tell one holder from another.
	lockTokenBytes = 16
)

// releaseScript deletes the lock only if it still holds our token. A plain DEL
// would free a lock that expired under us and was since taken by another
// replica — undoing exactly the exclusion the lock is for.
var releaseScript = goredis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0`)

// Lock takes key with SET NX PX and a random token, retrying briefly while it is
// held.
func (r *Repository) Lock(ctx context.Context, key string, ttl time.Duration) (func(context.Context), error) {
	token := make([]byte, lockTokenBytes)
	if _, err := rand.Read(token); err != nil {
		return nil, fmt.Errorf("generate a lock token: %w", err)
	}
	value := hex.EncodeToString(token)

	deadline := time.Now().Add(lockWait)
	for {
		taken, err := r.client.SetNX(ctx, key, value, ttl).Result()
		if err != nil {
			return nil, fmt.Errorf("take the lock %q: %w", key, err)
		}
		if taken {
			return func(ctx context.Context) {
				// Best effort: if this fails, the TTL frees the lock shortly anyway.
				_ = releaseScript.Run(ctx, r.client, []string{key}, value).Err()
			}, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: another change to the Redis users is in progress; try again", ErrBusy)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("wait for the lock %q: %w", key, ctx.Err())
		case <-time.After(lockRetry):
		}
	}
}

// Unlink removes a key without blocking on freeing its memory.
func (r *Repository) Unlink(ctx context.Context, key string) (int64, error) {
	removed, err := r.client.Unlink(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("unlink key: %w", err)
	}
	return removed, nil
}
