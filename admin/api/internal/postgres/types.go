package postgres

// Database is a database on the server.
type Database struct {
	Name string `json:"name"`
	// Owner is the role that owns it.
	Owner string `json:"owner"`
	// Encoding is the character set, e.g. UTF8.
	Encoding string `json:"encoding"`
	// SizeBytes is the on-disk size. Absent (0) when the connecting role cannot
	// read it, which is not an error worth failing a listing over.
	SizeBytes int64 `json:"sizeBytes"`
}

// Role is a PostgreSQL role: what other databases would call a user, except that
// a role without login is a group.
type Role struct {
	Name string `json:"name"`
	// CanLogin distinguishes a user from a group.
	CanLogin bool `json:"canLogin"`
	// IsSuperuser is reported so the panel can show which roles bypass every
	// permission check. The panel never creates one.
	IsSuperuser bool `json:"isSuperuser"`
	// CanCreateDatabase and CanCreateRole are the two privileges worth granting a
	// role that administers its own application.
	CanCreateDatabase bool `json:"canCreateDatabase"`
	CanCreateRole     bool `json:"canCreateRole"`
	// ConnectionLimit is -1 when unlimited, which is the default.
	ConnectionLimit int `json:"connectionLimit"`
}

// Extension is an extension installed in one database. Extensions are per
// database, not per server, which is why they hang off a database here.
type Extension struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// CreateDatabaseRequest asks for a new database.
type CreateDatabaseRequest struct {
	Name string `json:"name"`
	// Owner is the role that will own it. Empty leaves it owned by the role the
	// panel connects as.
	Owner string `json:"owner,omitempty"`
}

// CreateRoleRequest asks for a new role.
type CreateRoleRequest struct {
	Name string `json:"name"`
	// Password is required when CanLogin is true and must be empty otherwise —
	// a password on a role that cannot log in is refused rather than dropped, so
	// the caller is never left believing it set one. It never reaches the server
	// as plaintext; a SCRAM verifier is derived from it first.
	Password string `json:"password,omitempty"`
	// CanLogin makes the role a user rather than a group. Defaults to false, so
	// creating a login role is a deliberate act.
	CanLogin bool `json:"canLogin"`
	// CanCreateDatabase and CanCreateRole grant the two privileges an application
	// role sometimes needs. Superuser is deliberately not offered.
	CanCreateDatabase bool `json:"canCreateDatabase"`
	CanCreateRole     bool `json:"canCreateRole"`
}

// CreateExtensionRequest asks for an extension in one database.
type CreateExtensionRequest struct {
	Name string `json:"name"`
}

// UpdateRoleRequest is the desired state of a role.
//
// The whole state rather than a set of changes: the panel edits a role from a
// form showing its current values, so it knows all of them, and "set these three
// flags" needs no answer to what an omitted field means. A password is the one
// exception — it cannot be read back, so an empty one means "leave it alone"
// rather than "remove it".
type UpdateRoleRequest struct {
	CanLogin          bool `json:"canLogin"`
	CanCreateDatabase bool `json:"canCreateDatabase"`
	CanCreateRole     bool `json:"canCreateRole"`
	// ConnectionLimit is -1 for unlimited, which is PostgreSQL's own default.
	ConnectionLimit int `json:"connectionLimit"`
	// Password, when set, replaces the existing one. Never sent as plaintext: a
	// SCRAM verifier is derived from it first, the same as on create.
	Password string `json:"password,omitempty"`
}

// UpdateDatabaseRequest is the desired state of a database.
//
// Only the owner. Encoding cannot be changed after creation, and the name would
// be a rename — which breaks every connection string pointing at it, and is not
// something to offer from a form with no way to warn about that.
type UpdateDatabaseRequest struct {
	Owner string `json:"owner"`
}

// QueryRequest asks for rows from one database.
type QueryRequest struct {
	// SQL is a single read-only statement. It is not parsed or filtered here —
	// PostgreSQL enforces the read-only part itself. See Service.Query.
	SQL string `json:"sql"`
}

// QueryResult is what a query returned.
type QueryResult struct {
	// Columns are in the order the statement selected them.
	Columns []string `json:"columns"`
	// Rows hold values already rendered as strings. The panel displays them and
	// does not compute on them, and preserving every PostgreSQL type through JSON
	// would mean a type map that has to be kept current for no gain.
	Rows [][]string `json:"rows"`
	// Truncated reports that the result was cut at the row cap, so the panel can
	// say so rather than presenting a partial answer as the whole one.
	Truncated bool `json:"truncated"`
	// ElapsedMs is how long the server took, which is most of what a query is run
	// for when it is being tuned rather than read.
	ElapsedMs int64 `json:"elapsedMs"`
}
