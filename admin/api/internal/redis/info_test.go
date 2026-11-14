package redis

import (
	"reflect"
	"testing"
)

// A trimmed INFO reply in the shape Redis 8 sends it: CRLF line endings, section
// headers, and a keyspace line per non-empty database. The numbers are made up;
// the shape is what matters.
const sampleInfo = "# Server\r\n" +
	"redis_version:8.10.1\r\n" +
	"redis_mode:standalone\r\n" +
	"uptime_in_seconds:3600\r\n" +
	"\r\n" +
	"# Clients\r\n" +
	"connected_clients:4\r\n" +
	"blocked_clients:1\r\n" +
	"maxclients:10000\r\n" +
	"\r\n" +
	"# Memory\r\n" +
	"used_memory:1048576\r\n" +
	"used_memory_peak:2097152\r\n" +
	"maxmemory:134217728\r\n" +
	"maxmemory_policy:noeviction\r\n" +
	"\r\n" +
	"# Stats\r\n" +
	"total_commands_processed:500\r\n" +
	"rejected_connections:2\r\n" +
	"keyspace_hits:90\r\n" +
	"keyspace_misses:10\r\n" +
	"expired_keys:7\r\n" +
	"evicted_keys:0\r\n" +
	"\r\n" +
	"# Errorstats\r\n" +
	"errorstat_ERR:count=3\r\n" +
	"errorstat_OOM:count=12\r\n" +
	"\r\n" +
	"# Keyspace\r\n" +
	"db0:keys=42,expires=40,avg_ttl=1000,subexpiry=0\r\n" +
	"db3:keys=1,expires=0,avg_ttl=0,subexpiry=0\r\n"

func TestParseInfo(t *testing.T) {
	tests := []struct {
		name string
		info string
		want Overview
	}{
		{
			name: "a full reply maps every field we show",
			info: sampleInfo,
			want: Overview{
				Version: "8.10.1", Mode: "standalone", UptimeSeconds: 3600,
				UsedMemoryBytes: 1048576, PeakMemoryBytes: 2097152,
				MaxMemoryBytes: 134217728, MaxMemoryPolicy: "noeviction",
				ConnectedClients: 4, BlockedClients: 1, MaxClients: 10000, RejectedConnections: 2,
				CommandsProcessed: 500, KeyspaceHits: 90, KeyspaceMisses: 10,
				ExpiredKeys: 7, OOMRejections: 12,
				Keyspace: []Keyspace{
					{Database: 0, Keys: 42, Expires: 40},
					{Database: 3, Keys: 1, Expires: 0},
				},
			},
		},
		{
			// A server that never refused a write has no errorstat_OOM line at all,
			// and an empty one has no keyspace lines. Neither is an error: both are
			// zero, and the keyspace is an empty list rather than null so the panel
			// can render "no keys" without a special case.
			name: "absent lines are zero rather than a failure",
			info: "# Server\nredis_version:8.10.1\n",
			want: Overview{Version: "8.10.1", Keyspace: []Keyspace{}},
		},
		{
			// Bare LF as well as CRLF, and a value that is not a number. We would
			// rather show a zero than refuse the whole overview over one field a
			// future version renamed or reformatted.
			name: "unparseable numbers degrade to zero",
			info: "used_memory:lots\nconnected_clients:3\n",
			want: Overview{ConnectedClients: 3, Keyspace: []Keyspace{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseInfo(tt.info)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseInfo() =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}
