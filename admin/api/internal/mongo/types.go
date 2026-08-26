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
