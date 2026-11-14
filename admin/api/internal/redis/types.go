package redis

// Overview is what the server says about itself, reduced to what an operator
// looks at when the coordination point is misbehaving.
//
// Memory is the headline. The platform runs this Redis with `noeviction`, so
// reaching MaxMemoryBytes does not drop old keys — it refuses every write, and
// the first thing to notice it is the panel's own token refresh. OOMRejections
// counts the writes that already met that wall.
type Overview struct {
	Version       string `json:"version"`
	Mode          string `json:"mode"`
	UptimeSeconds int64  `json:"uptimeSeconds"`

	UsedMemoryBytes int64 `json:"usedMemoryBytes"`
	PeakMemoryBytes int64 `json:"peakMemoryBytes"`
	// MaxMemoryBytes is zero when the server has no ceiling at all.
	MaxMemoryBytes  int64  `json:"maxMemoryBytes"`
	MaxMemoryPolicy string `json:"maxMemoryPolicy"`

	ConnectedClients    int64 `json:"connectedClients"`
	BlockedClients      int64 `json:"blockedClients"`
	MaxClients          int64 `json:"maxClients"`
	RejectedConnections int64 `json:"rejectedConnections"`

	CommandsProcessed int64 `json:"commandsProcessed"`
	KeyspaceHits      int64 `json:"keyspaceHits"`
	KeyspaceMisses    int64 `json:"keyspaceMisses"`
	ExpiredKeys       int64 `json:"expiredKeys"`
	EvictedKeys       int64 `json:"evictedKeys"`
	// OOMRejections is the number of commands refused for want of memory since
	// the server started.
	OOMRejections int64 `json:"oomRejections"`

	Keyspace []Keyspace `json:"keyspace"`
}

// Keyspace is one logical database that holds anything. Redis omits empty ones.
type Keyspace struct {
	Database int   `json:"database"`
	Keys     int64 `json:"keys"`
	Expires  int64 `json:"expires"`
}

// Text is a value that came out of Redis, which stores bytes rather than
// strings.
//
// Most of what lands here is readable — lock names, JSON, counters — and is
// returned as is. Anything that is not valid UTF-8 is returned base64 with
// Encoding saying so, because putting it in a JSON string as it stands either
// mangles it or breaks the response.
type Text struct {
	Value string `json:"value"`
	// Encoding is "utf8" or "base64".
	Encoding string `json:"encoding"`
}

// The two encodings a Text can carry.
const (
	encodingUTF8   = "utf8"
	encodingBase64 = "base64"
)

// KeySummary is one key in a listing: enough to decide whether to open it.
type KeySummary struct {
	Key  Text   `json:"key"`
	Type string `json:"type"`
	// TTLMillis is the time left before the key expires, or -1 for a key that
	// never does. A key that expired between the scan and this read is dropped
	// from the page rather than reported with a TTL that means "gone".
	TTLMillis int64 `json:"ttlMillis"`
}

// KeyPage is one page of a scan.
//
// Redis's SCAN guarantees every key present for the whole iteration is returned
// at least once, and nothing about page size: a page can be empty while the
// cursor is not done. NextCursor is empty when the iteration is over, and only
// then.
type KeyPage struct {
	Keys       []KeySummary `json:"keys"`
	NextCursor string       `json:"nextCursor"`
}

// Field is one name/value pair from a hash or a stream entry.
type Field struct {
	Name  Text `json:"name"`
	Value Text `json:"value"`
}

// ScoredMember is one member of a sorted set.
type ScoredMember struct {
	Member Text    `json:"member"`
	Score  float64 `json:"score"`
}

// StreamEntry is one entry of a stream.
type StreamEntry struct {
	ID     string  `json:"id"`
	Fields []Field `json:"fields"`
}

// KeyDetail is one key opened up.
//
// The value is BOUNDED. A list can hold millions of items and a string half a
// gigabyte; this returns the first items and the first bytes, and Truncated says
// so. Exactly one of the value fields is set, the one matching Type — and none
// for a type this panel does not know how to read, such as a module's.
type KeyDetail struct {
	Key         Text   `json:"key"`
	Type        string `json:"type"`
	TTLMillis   int64  `json:"ttlMillis"`
	MemoryBytes int64  `json:"memoryBytes"`
	// Length is the full size: bytes for a string, items for everything else.
	Length    int64 `json:"length"`
	Truncated bool  `json:"truncated"`
	// Supported is false for a type the panel shows but cannot read.
	Supported bool `json:"supported"`

	String  *Text          `json:"string,omitempty"`
	Items   []Text         `json:"items,omitempty"`
	Fields  []Field        `json:"fields,omitempty"`
	Members []ScoredMember `json:"members,omitempty"`
	Entries []StreamEntry  `json:"entries,omitempty"`
}
