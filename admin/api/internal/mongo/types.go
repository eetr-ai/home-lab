package mongo

// Database is a database on the server.
type Database struct {
	Name string `json:"name"`
	// SizeBytes is the size on disk as the server reports it.
	SizeBytes int64 `json:"sizeBytes"`
	// Empty is true for a database the server lists but which holds nothing.
	Empty bool `json:"empty"`
}

// Collection is a collection within one database.
type Collection struct {
	Name string `json:"name"`
	// Type is "collection" or "view".
	Type string `json:"type"`
}

// User is an account on the server. MongoDB scopes a user to the database it was
// created in, which is why Database is part of its identity rather than a detail.
type User struct {
	Name     string `json:"name"`
	Database string `json:"database"`
	Roles    []Role `json:"roles"`
}

// Role is one grant: a named role on a named database.
type Role struct {
	Name     string `json:"name"`
	Database string `json:"database"`
}

// CreateDatabaseRequest asks for a new database.
//
// MongoDB has no standalone create-database command: a database begins to exist
// when something is put in it. Naming the first collection is therefore part of
// the request rather than a convenience — without it there would be nothing to
// create, and a call that appeared to succeed would leave no database behind.
type CreateDatabaseRequest struct {
	Name string `json:"name"`
	// Collection is created inside the new database, bringing it into existence.
	Collection string `json:"collection"`
}

// CreateCollectionRequest asks for a new collection in an existing database.
type CreateCollectionRequest struct {
	Name string `json:"name"`
}

// CreateUserRequest asks for a new user.
type CreateUserRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	// Roles granted to the user. Each names a role and the database it applies
	// to; an empty Database on a role means the database the user is created in.
	Roles []Role `json:"roles"`
}

// UpdateUserRequest is the desired state of a user.
//
// Roles and, optionally, a password. The whole role set rather than a set of
// grants and revocations: MongoDB's updateUser replaces the array outright, and
// the panel edits from a form showing the current roles, so it knows all of them.
type UpdateUserRequest struct {
	Roles []Role `json:"roles"`
	// Password, when set, replaces the existing one. Empty leaves it alone — a
	// password cannot be read back, so an edit form that omits it means "unchanged"
	// rather than "remove".
	Password string `json:"password,omitempty"`
}

// FindRequest asks for documents from one collection.
//
// A find, not a command. Filter, projection and sort are the parts of a query an
// operator reaches for when looking at data, and each is a document rather than a
// string — so nothing here is parsed as syntax.
type FindRequest struct {
	Collection string `json:"collection"`
	// Filter is a MongoDB query document. Empty matches everything.
	Filter map[string]any `json:"filter,omitempty"`
	// Projection selects fields. Empty returns whole documents.
	Projection map[string]any `json:"projection,omitempty"`
	// Sort orders the results, e.g. {"createdAt": -1}.
	Sort map[string]any `json:"sort,omitempty"`
	// Limit caps the documents returned, up to the server-side maximum.
	Limit int64 `json:"limit,omitempty"`
}

// FindResult is what a find returned.
type FindResult struct {
	// Documents are rendered as extended-JSON strings, one per document. The panel
	// displays them; carrying BSON's types faithfully through JSON would mean a
	// type map to keep current for no gain.
	Documents []string `json:"documents"`
	// Truncated reports that the result was cut at the limit, so the panel can say
	// so rather than presenting a partial answer as the whole one.
	Truncated bool  `json:"truncated"`
	ElapsedMs int64 `json:"elapsedMs"`
}
