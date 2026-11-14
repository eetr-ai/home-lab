package redis

import (
	"strconv"
	"strings"
)

// parseInfo turns the text of an INFO reply into an Overview.
//
// INFO is a list of `name:value` lines grouped under `# Section` headers, which
// we can ignore because every name is unique across sections. It is a format
// Redis adds to between versions, so this is deliberately forgiving: a line we do
// not know is skipped and a number we cannot read is zero. Refusing the whole
// overview because one counter changed shape would be the wrong trade for a page
// whose job is to be there when something is already wrong.
func parseInfo(info string) Overview {
	fields := map[string]string{}
	ret := Overview{Keyspace: []Keyspace{}}

	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}

		// db0, db1, ... are the keyspace lines, and the only ones whose value is a
		// list of its own rather than a scalar.
		if database, ok := databaseIndex(name); ok {
			ret.Keyspace = append(ret.Keyspace, parseKeyspace(database, value))
			continue
		}
		fields[name] = value
	}

	number := func(name string) int64 {
		n, _ := strconv.ParseInt(fields[name], 10, 64)
		return n
	}

	ret.Version = fields["redis_version"]
	ret.Mode = fields["redis_mode"]
	ret.UptimeSeconds = number("uptime_in_seconds")

	ret.UsedMemoryBytes = number("used_memory")
	ret.PeakMemoryBytes = number("used_memory_peak")
	ret.MaxMemoryBytes = number("maxmemory")
	ret.MaxMemoryPolicy = fields["maxmemory_policy"]

	ret.ConnectedClients = number("connected_clients")
	ret.BlockedClients = number("blocked_clients")
	ret.MaxClients = number("maxclients")
	ret.RejectedConnections = number("rejected_connections")

	ret.CommandsProcessed = number("total_commands_processed")
	ret.KeyspaceHits = number("keyspace_hits")
	ret.KeyspaceMisses = number("keyspace_misses")
	ret.ExpiredKeys = number("expired_keys")
	ret.EvictedKeys = number("evicted_keys")

	// The errorstats section counts error replies by their prefix, and OOM is the
	// one `noeviction` produces when a write meets the ceiling. Its value is a
	// small list of its own, `count=12`.
	ret.OOMRejections = subfield(fields["errorstat_OOM"], "count")

	return ret
}

// databaseIndex reports whether name is a keyspace line, and for which database.
func databaseIndex(name string) (int, bool) {
	digits, found := strings.CutPrefix(name, "db")
	if !found || digits == "" {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0, false
	}
	return n, true
}

// parseKeyspace reads `keys=42,expires=40,avg_ttl=1000`.
func parseKeyspace(database int, value string) Keyspace {
	return Keyspace{
		Database: database,
		Keys:     subfield(value, "keys"),
		Expires:  subfield(value, "expires"),
	}
}

// subfield pulls one number out of a comma-separated `name=value` list.
func subfield(list, name string) int64 {
	for _, pair := range strings.Split(list, ",") {
		key, value, found := strings.Cut(pair, "=")
		if found && key == name {
			n, _ := strconv.ParseInt(value, 10, 64)
			return n
		}
	}
	return 0
}
