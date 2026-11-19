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

// Column is one column of a table or view.
type Column struct {
	Name string `json:"name"`
	// Type is the type as PostgreSQL renders it — format_type output, e.g.
	// "integer" or "character varying(255)" — so the panel shows the declared type
	// rather than an OID it would have to map itself.
	Type string `json:"type"`
	// Nullable is false for a NOT NULL column.
	Nullable bool `json:"nullable"`
	// PrimaryKey marks a column that is part of the table's primary key, which is
	// what makes stable keyset paging possible when browsing it.
	PrimaryKey bool `json:"primaryKey"`
}

// Relation is a table or view the console can list and browse.
type Relation struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	// Kind is "table", "view", or "matview". A view has no primary key, so it
	// browses as a single capped page rather than paginating.
	Kind    string   `json:"kind"`
	Columns []Column `json:"columns"`
}

// BrowseRequest asks for one page of a table or view.
//
// The schema and table name the relation; the cursor continues from a previous
// page. The primary key it pages over is not carried here — the server reads it
// from the catalog, so a caller cannot ask to order by a column that is not the
// key and quietly get an unstable page.
type BrowseRequest struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
	// Cursor is the opaque NextCursor from the previous page. Empty starts at the
	// first page.
	Cursor string `json:"cursor,omitempty"`
	// PageSize is how many rows a page holds. Zero means the default; larger than
	// the cap is clamped down to it.
	PageSize int `json:"pageSize,omitempty"`
}

// BrowseResult is one page of a table.
type BrowseResult struct {
	// Columns and Rows are the page's data, rendered the same way a query's are.
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
	// NextCursor continues from the last row of this page. Empty when there is no
	// next page, or when the relation has no primary key to page over.
	NextCursor string `json:"nextCursor,omitempty"`
	// Truncated reports a relation with no primary key whose rows did not all fit
	// in one page — it cannot be paged, so the extra rows are simply not shown,
	// the same as the query console's row cap.
	Truncated bool `json:"truncated"`
	// SQL is the readable statement this page corresponds to, for the console to
	// show in its editor. It is the base query without the cursor bookkeeping, so
	// it stays the same across pages and is a statement the operator can run and
	// edit for themselves.
	SQL string `json:"sql"`
	// EstimatedRows is PostgreSQL's own estimate of the relation's live row count
	// (pg_class.reltuples), so the pager can show an approximate page total without
	// a COUNT(*) scan. Zero when it is unknown — a view, or a table never analyzed.
	EstimatedRows int64 `json:"estimatedRows"`
	// ElapsedMs is how long the server took to return the page.
	ElapsedMs int64 `json:"elapsedMs"`
}

// QueryRequest asks for rows from one database.
type QueryRequest struct {
	// SQL is a single read-only statement. It is not parsed or filtered here —
	// PostgreSQL enforces the read-only part itself. See Service.Query.
	SQL string `json:"sql"`
}

// ExecuteResult is what a modifying statement did.
//
// It carries the same rendered rows a query does — a statement with a RETURNING
// clause has some — plus what a read never has: the command tag and the number of
// rows it changed, which is the whole answer for an INSERT, UPDATE or DELETE that
// returns nothing.
type ExecuteResult struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
	// Truncated reports RETURNING rows cut at the cap. The statement still ran in
	// full and RowsAffected is exact; only the rows shown back are capped.
	Truncated bool `json:"truncated"`
	// Command is PostgreSQL's own tag, e.g. "UPDATE 3" or "CREATE TABLE".
	Command string `json:"command"`
	// RowsAffected is how many rows the statement changed, from the command tag.
	RowsAffected int64 `json:"rowsAffected"`
	ElapsedMs    int64 `json:"elapsedMs"`
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

// ObjectPrivileges is what one role holds on one object: the database itself, a
// schema, a table, or a sequence.
//
// Two lists rather than one, because they answer different questions and the
// difference is exactly what an operator trips over. Granted is what was granted
// to this role by name — what a REVOKE from it can take away. Effective is what
// the role can actually do, which also counts privileges granted to PUBLIC and to
// any role it inherits from. A privilege in Effective but not Granted cannot be
// revoked here; it has to be revoked where it was granted.
type ObjectPrivileges struct {
	// Schema is empty for the database and for a schema itself.
	Schema string `json:"schema,omitempty"`
	Name   string `json:"name"`
	// Kind is "database", "schema", "table", "view", "matview", "foreign", or
	// "sequence". A view is granted to with the same statement as a table, so the
	// distinction is only for display.
	Kind string `json:"kind"`
	// Owner is the object's owner. An owner is granted every privilege by
	// default, so for an object the role owns, Granted lists all of them unless the
	// owner has revoked some from itself — which it can, and can grant back.
	Owner   string   `json:"owner"`
	Granted []string `json:"granted"`
	// ForeignGrants are the entries of Granted that a role other than the owner
	// granted. The panel's revoke acts as the owner and cannot remove them, so
	// they are reported for the caller to leave alone, not to offer as a revoke.
	ForeignGrants []ForeignGrant `json:"foreignGrants"`
	Effective     []string       `json:"effective"`
}

// ForeignGrant is one privilege granted to the role by somebody other than the
// object's owner — a role that held it WITH GRANT OPTION.
type ForeignGrant struct {
	Privilege string `json:"privilege"`
	Grantor   string `json:"grantor"`
}

// DefaultPrivileges is one ALTER DEFAULT PRIVILEGES entry naming the role.
//
// It is not a privilege on anything that exists. It says that objects created
// later, by Owner, in Schema, will be created with these privileges granted to
// the role — and only objects created by that owner: a table someone else
// creates is not covered, which is the mistake this type's fields exist to make
// visible.
type DefaultPrivileges struct {
	Owner string `json:"owner"`
	// Schema is empty for an entry that applies in every schema.
	Schema string `json:"schema,omitempty"`
	// ObjectType is "tables" or "sequences".
	ObjectType string   `json:"objectType"`
	Privileges []string `json:"privileges"`
}

// RolePrivileges is everything one role holds in one database.
//
// Per database because privileges are: a table's ACL lives in that database's
// catalog, and the same role can hold quite different things in two databases.
type RolePrivileges struct {
	Role     string `json:"role"`
	Database string `json:"database"`
	// IsSuperuser is reported because a superuser bypasses every privilege check:
	// every Effective list is full, and granting to it changes nothing.
	IsSuperuser bool `json:"isSuperuser"`
	// DatabasePrivileges is CONNECT, CREATE and TEMPORARY on the database itself.
	DatabasePrivileges ObjectPrivileges    `json:"databasePrivileges"`
	Schemas            []ObjectPrivileges  `json:"schemas"`
	Tables             []ObjectPrivileges  `json:"tables"`
	Sequences          []ObjectPrivileges  `json:"sequences"`
	Defaults           []DefaultPrivileges `json:"defaults"`
}

// PrivilegeChange is one GRANT or REVOKE.
//
// Target says what kind of object it applies to, and the other fields are read
// according to it — and refused when they do not apply, rather than ignored, so a
// caller who sent Objects with "allTables" learns that the list meant nothing:
//
//   - "database": the database in the path. No Schema, no Objects.
//   - "schema": Schema itself.
//   - "table", "sequence": each of Objects, in Schema.
//   - "allTables", "allSequences": every one that exists in Schema now. Not ones
//     created later — that is what the default targets are for.
//   - "defaultTables", "defaultSequences": ALTER DEFAULT PRIVILEGES for objects
//     Owner creates later, in Schema, or in every schema when Schema is empty.
type PrivilegeChange struct {
	// Action is "grant" or "revoke".
	Action string `json:"action"`
	Target string `json:"target"`
	Schema string `json:"schema,omitempty"`
	// Objects are table or sequence names within Schema.
	Objects []string `json:"objects,omitempty"`
	// Privileges are keywords valid for the target, e.g. SELECT for a table or
	// USAGE for a sequence. ALL is not accepted: it means different things on
	// different PostgreSQL versions, and the panel shows a fixed set.
	Privileges []string `json:"privileges"`
	// Owner is the role whose future objects a default target covers.
	Owner string `json:"owner,omitempty"`
}

// ChangePrivilegesRequest is a set of changes applied together.
//
// A list because the useful changes come in groups — read-write access to a
// schema is USAGE on it, four privileges on its tables and two on its sequences —
// and the group is applied in one transaction, so it lands whole or not at all.
type ChangePrivilegesRequest struct {
	Changes []PrivilegeChange `json:"changes"`
}

// ChangePrivilegesResult is what a change ran and where it left the role.
type ChangePrivilegesResult struct {
	// Statements are the GRANT and REVOKE statements that were committed, in
	// order, so the caller can see — and keep — exactly what was done.
	Statements []string `json:"statements"`
	// Privileges is the role's state in the database after the change.
	Privileges RolePrivileges `json:"privileges"`
}
