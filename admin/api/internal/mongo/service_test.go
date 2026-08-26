package mongo

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeRepo stands in for MongoDB, recording what it was asked to do so a test can
// check that a refusal happened before anything reached the server.
type fakeRepo struct {
	databases   map[string]bool
	collections map[string]map[string]bool
	users       map[string]map[string]bool
	current     string

	created []string
	dropped []string
	updated []string
	lastReq CreateUserRequest
	err     error

	lastUserUpdate UpdateUserRequest
	lastFind       string
	findResult     FindResult
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		databases:   map[string]bool{"admin": true, "config": true, "local": true, "app": true},
		collections: map[string]map[string]bool{"app": {"users": true}},
		users:       map[string]map[string]bool{"admin": {"root_account": true}},
		current:     "root_account",
	}
}

func (f *fakeRepo) ListDatabases(context.Context) ([]Database, error) {
	out := make([]Database, 0, len(f.databases))
	for name := range f.databases {
		out = append(out, Database{Name: name})
	}
	return out, f.err
}

func (f *fakeRepo) DropDatabase(_ context.Context, name string) error {
	f.dropped = append(f.dropped, "database:"+name)
	delete(f.databases, name)
	return f.err
}

func (f *fakeRepo) DatabaseExists(_ context.Context, name string) (bool, error) {
	return f.databases[name], f.err
}

func (f *fakeRepo) ListCollections(_ context.Context, database string) ([]Collection, error) {
	out := make([]Collection, 0, len(f.collections[database]))
	for name := range f.collections[database] {
		out = append(out, Collection{Name: name, Type: "collection"})
	}
	return out, f.err
}

func (f *fakeRepo) CreateCollection(_ context.Context, database, name string) error {
	f.created = append(f.created, "collection:"+database+"."+name)
	if f.collections[database] == nil {
		f.collections[database] = map[string]bool{}
	}
	f.collections[database][name] = true
	f.databases[database] = true
	return f.err
}

func (f *fakeRepo) DropCollection(_ context.Context, database, name string) error {
	f.dropped = append(f.dropped, "collection:"+database+"."+name)
	delete(f.collections[database], name)
	return f.err
}

func (f *fakeRepo) CollectionExists(_ context.Context, database, name string) (bool, error) {
	return f.collections[database][name], f.err
}

func (f *fakeRepo) ListUsers(_ context.Context, database string) ([]User, error) {
	out := make([]User, 0, len(f.users[database]))
	for name := range f.users[database] {
		out = append(out, User{Name: name, Database: database})
	}
	return out, f.err
}

func (f *fakeRepo) CreateUser(_ context.Context, database string, req CreateUserRequest) error {
	f.created = append(f.created, "user:"+database+"."+req.Name)
	f.lastReq = req
	if f.users[database] == nil {
		f.users[database] = map[string]bool{}
	}
	f.users[database][req.Name] = true
	return f.err
}

func (f *fakeRepo) DropUser(_ context.Context, database, name string) error {
	f.dropped = append(f.dropped, "user:"+database+"."+name)
	delete(f.users[database], name)
	return f.err
}

func (f *fakeRepo) UserExists(_ context.Context, database, name string) (bool, error) {
	return f.users[database][name], f.err
}

func (f *fakeRepo) UpdateUser(_ context.Context, database, name string, req UpdateUserRequest) error {
	f.updated = append(f.updated, database+"."+name)
	f.lastUserUpdate = req
	return f.err
}

func (f *fakeRepo) Find(_ context.Context, database string, req FindRequest) (FindResult, error) {
	f.lastFind = database + "." + req.Collection
	return f.findResult, f.err
}

func (f *fakeRepo) CurrentUser(context.Context) (string, error) {
	return f.current, f.err
}

const goodPassword = "a-long-enough-password"

func TestCreateDatabase(t *testing.T) {
	tests := []struct {
		name    string
		request CreateDatabaseRequest
		wantErr error
	}{
		{name: "a new database", request: CreateDatabaseRequest{Name: "orders", Collection: "items"}},
		{name: "an invalid database name", request: CreateDatabaseRequest{Name: "orders.x", Collection: "items"}, wantErr: ErrInvalidName},
		{name: "an invalid collection name", request: CreateDatabaseRequest{Name: "orders", Collection: "system.items"}, wantErr: ErrInvalidName},
		{name: "no collection at all", request: CreateDatabaseRequest{Name: "orders"}, wantErr: ErrInvalidName},
		{name: "one of MongoDB's own", request: CreateDatabaseRequest{Name: "admin", Collection: "x"}, wantErr: ErrProtected},
		{name: "a name already taken", request: CreateDatabaseRequest{Name: "app", Collection: "x"}, wantErr: ErrAlreadyExists},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepo()
			service := NewService(repo)

			_, err := service.CreateDatabase(t.Context(), test.request)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("CreateDatabase() error = %v, want %v", err, test.wantErr)
				}
				if len(repo.created) != 0 {
					t.Errorf("CreateDatabase() was refused but still called the repository: %v", repo.created)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateDatabase() error = %v", err)
			}
			// A database exists only once something is in it, so the collection
			// is the creation — not an extra step that could be skipped.
			if len(repo.created) != 1 || repo.created[0] != "collection:orders.items" {
				t.Errorf("CreateDatabase() calls = %v, want the initial collection created", repo.created)
			}
		})
	}
}

func TestDropDatabase(t *testing.T) {
	tests := []struct {
		name     string
		database string
		wantErr  error
	}{
		{name: "an ordinary database", database: "app"},
		{name: "admin", database: "admin", wantErr: ErrProtected},
		{name: "config", database: "config", wantErr: ErrProtected},
		{name: "local", database: "local", wantErr: ErrProtected},
		{name: "a protected name in mixed case", database: "Admin", wantErr: ErrProtected},
		{name: "one that does not exist", database: "missing", wantErr: ErrNotFound},
		{name: "an invalid name", database: "app$", wantErr: ErrInvalidName},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepo()
			service := NewService(repo)

			err := service.DropDatabase(t.Context(), test.database)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("DropDatabase(%q) error = %v, want %v", test.database, err, test.wantErr)
				}
				if len(repo.dropped) != 0 {
					t.Errorf("DropDatabase(%q) was refused but still called the repository", test.database)
				}
				return
			}
			if err != nil {
				t.Fatalf("DropDatabase(%q) error = %v", test.database, err)
			}
		})
	}
}

func TestCreateUser(t *testing.T) {
	readWrite := []Role{{Name: "readWrite", Database: "app"}}

	tests := []struct {
		name    string
		request CreateUserRequest
		wantErr error
	}{
		{name: "a user with a scoped role", request: CreateUserRequest{Name: "app_user", Password: goodPassword, Roles: readWrite}},
		{name: "an invalid user name", request: CreateUserRequest{Name: "app$", Password: goodPassword, Roles: readWrite}, wantErr: ErrInvalidName},
		{name: "a short password", request: CreateUserRequest{Name: "app_user", Password: "short", Roles: readWrite}, wantErr: ErrWeakPassword},
		// A user with no roles authenticates and can do nothing, which is almost
		// never what was meant and is tedious to diagnose.
		{name: "no roles", request: CreateUserRequest{Name: "app_user", Password: goodPassword}, wantErr: ErrInvalidRole},
		// Superuser-equivalent roles are not on offer, the same as PostgreSQL.
		{name: "root", request: CreateUserRequest{Name: "u", Password: goodPassword, Roles: []Role{{Name: "root", Database: "admin"}}}, wantErr: ErrInvalidRole},
		{name: "readWriteAnyDatabase", request: CreateUserRequest{Name: "u", Password: goodPassword, Roles: []Role{{Name: "readWriteAnyDatabase", Database: "admin"}}}, wantErr: ErrInvalidRole},
		{name: "backup", request: CreateUserRequest{Name: "u", Password: goodPassword, Roles: []Role{{Name: "backup", Database: "admin"}}}, wantErr: ErrInvalidRole},
		{name: "a role on an invalid database", request: CreateUserRequest{Name: "u", Password: goodPassword, Roles: []Role{{Name: "readWrite", Database: "a$b"}}}, wantErr: ErrInvalidName},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepo()
			service := NewService(repo)

			_, err := service.CreateUser(t.Context(), "app", test.request)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("CreateUser() error = %v, want %v", err, test.wantErr)
				}
				if len(repo.created) != 0 {
					t.Errorf("CreateUser() was refused but still called the repository")
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateUser() error = %v", err)
			}
			if repo.lastReq.Name != test.request.Name || len(repo.lastReq.Roles) != len(test.request.Roles) {
				t.Errorf("CreateUser() passed %+v, want %+v", repo.lastReq, test.request)
			}
		})
	}
}

func TestDropUserRefusesTheConnectingAccount(t *testing.T) {
	for _, name := range []string{"root_account", "ROOT_ACCOUNT", "Root_Account"} {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.users["admin"][name] = true
			service := NewService(repo)

			err := service.DropUser(t.Context(), "admin", name)
			if !errors.Is(err, ErrProtected) {
				t.Fatalf("DropUser(%q) error = %v, want %v", name, err, ErrProtected)
			}
			if len(repo.dropped) != 0 {
				t.Errorf("DropUser(%q) was refused but still called the repository", name)
			}
		})
	}
}

func TestDropCollection(t *testing.T) {
	tests := []struct {
		name       string
		database   string
		collection string
		wantErr    error
	}{
		{name: "an ordinary collection", database: "app", collection: "users"},
		{name: "one that does not exist", database: "app", collection: "missing", wantErr: ErrNotFound},
		{name: "a reserved collection", database: "app", collection: "system.users", wantErr: ErrInvalidName},
		// Even a legitimately named collection is refused inside MongoDB's own
		// databases; admin holds every credential on the server.
		{name: "inside admin", database: "admin", collection: "users", wantErr: ErrProtected},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.collections["admin"] = map[string]bool{"users": true}
			service := NewService(repo)

			err := service.DropCollection(t.Context(), test.database, test.collection)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("DropCollection() error = %v, want %v", err, test.wantErr)
				}
				if len(repo.dropped) != 0 {
					t.Errorf("DropCollection() was refused but still called the repository")
				}
				return
			}
			if err != nil {
				t.Fatalf("DropCollection() error = %v", err)
			}
		})
	}
}

// Creating a collection inside MongoDB's own databases has to be refused for the
// same reason dropping one is: admin holds every credential on the server, and a
// collection added there is a write to the thing the panel authenticates against.
func TestCreateCollectionRefusesProtectedDatabases(t *testing.T) {
	for _, database := range []string{"admin", "config", "local", "Admin"} {
		t.Run(database, func(t *testing.T) {
			repo := newFakeRepo()
			service := NewService(repo)

			_, err := service.CreateCollection(t.Context(), database, CreateCollectionRequest{Name: "notes"})
			if !errors.Is(err, ErrProtected) {
				t.Fatalf("CreateCollection(%q) error = %v, want %v", database, err, ErrProtected)
			}
			if len(repo.created) != 0 {
				t.Errorf("CreateCollection(%q) was refused but still called the repository", database)
			}
		})
	}
}

// The account the panel connects as must be untouchable. Revoking its own roles
// through a form would lock the panel out of the server it administers — the same
// reason it cannot be dropped.
func TestUpdateUserRefusesTheConnectingAccount(t *testing.T) {
	repo := newFakeRepo()

	// Case-insensitively, matching the drop path. The same invariant spelled two
	// ways in one file is how a name differing only in case ends up refused by
	// one and accepted by the other.
	for _, name := range []string{"root_account", "ROOT_ACCOUNT", "Root_Account"} {
		_, err := NewService(repo).UpdateUser(t.Context(), "admin", name,
			UpdateUserRequest{Roles: []Role{{Name: "read", Database: "app"}}})
		if !errors.Is(err, ErrProtected) {
			t.Fatalf("UpdateUser(%q) error = %v, want %v", name, err, ErrProtected)
		}
	}
	if len(repo.updated) != 0 {
		t.Errorf("a refused update still reached the server: %v", repo.updated)
	}
}

func TestUpdateUserValidation(t *testing.T) {
	tests := []struct {
		name    string
		user    string
		request UpdateUserRequest
		wantErr error
	}{
		{
			name:    "an ordinary update",
			user:    "app_user",
			request: UpdateUserRequest{Roles: []Role{{Name: "readWrite", Database: "app"}}},
		},
		{
			name: "a password reset alongside the roles",
			user: "app_user",
			request: UpdateUserRequest{
				Roles:    []Role{{Name: "read", Database: "app"}},
				Password: strings.Repeat("x", 16),
			},
		},
		{
			// A user with no roles can authenticate and do nothing, which is almost
			// never what was meant and is confusing to diagnose.
			name: "no roles at all", user: "app_user",
			request: UpdateUserRequest{Roles: []Role{}}, wantErr: ErrInvalidRole,
		},
		{
			name: "a role that administers the whole server", user: "app_user",
			request: UpdateUserRequest{Roles: []Role{{Name: "root", Database: "admin"}}},
			wantErr: ErrInvalidRole,
		},
		{
			name: "a short password", user: "app_user",
			request: UpdateUserRequest{
				Roles: []Role{{Name: "read", Database: "app"}}, Password: "short",
			},
			wantErr: ErrWeakPassword,
		},
		{
			name: "a user that does not exist", user: "absent",
			request: UpdateUserRequest{Roles: []Role{{Name: "read", Database: "app"}}},
			wantErr: ErrNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.users["admin"]["app_user"] = true

			_, err := NewService(repo).UpdateUser(t.Context(), "admin", test.user, test.request)

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("error = %v, want %v", err, test.wantErr)
				}
				if len(repo.updated) != 0 {
					t.Errorf("a refused update still reached the server: %v", repo.updated)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if len(repo.updated) != 1 {
				t.Errorf("updated = %v, want exactly one", repo.updated)
			}
			// The request itself, not just that a call happened: a service that
			// reshaped the role set or dropped the password would pass a count.
			if !reflect.DeepEqual(repo.lastUserUpdate, test.request) {
				t.Errorf("the request was reshaped: got %+v, want %+v",
					repo.lastUserUpdate, test.request)
			}
		})
	}
}

// $where and $function run arbitrary JavaScript inside the database with the
// panel's own credentials, and MongoDB has no read-only mode to fall back on —
// unlike PostgreSQL, where the engine refuses writes itself. This check is the
// whole of that boundary, so it has to hold at every depth: a $where nested
// inside an $and inside an $or is the same instruction to the server as one at
// the top.
func TestFindRefusesServerSideJavaScript(t *testing.T) {
	tests := []struct {
		name    string
		filter  map[string]any
		refused bool
	}{
		{
			name:   "an ordinary filter",
			filter: map[string]any{"status": "active", "count": map[string]any{"$gt": 5}},
		},
		{
			// $expr evaluates aggregation expressions, not JavaScript. Refusing it
			// would break ordinary queries for no gain.
			name:   "$expr is not JavaScript and is allowed",
			filter: map[string]any{"$expr": map[string]any{"$gt": []any{"$a", "$b"}}},
		},
		{
			name:    "$where at the top level",
			filter:  map[string]any{"$where": "this.a > 1"},
			refused: true,
		},
		{
			name: "$where nested inside $and",
			filter: map[string]any{
				"$and": []any{
					map[string]any{"status": "active"},
					map[string]any{"$where": "this.a > 1"},
				},
			},
			refused: true,
		},
		{
			name: "$function deep inside an $or of $ands",
			filter: map[string]any{
				"$or": []any{
					map[string]any{"$and": []any{
						map[string]any{"$function": map[string]any{"body": "function(){}"}},
					}},
				},
			},
			refused: true,
		},
		{
			name:    "$accumulator",
			filter:  map[string]any{"$accumulator": map[string]any{"init": "function(){}"}},
			refused: true,
		},
		{
			// Case is not a defence: MongoDB matches operators case-sensitively, but
			// a check that did not fold case would invite exactly one attempt.
			name:    "$WHERE in a different case",
			filter:  map[string]any{"$WHERE": "this.a > 1"},
			refused: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepo()
			_, err := NewService(repo).Find(t.Context(), "app",
				FindRequest{Collection: "users", Filter: test.filter})

			if test.refused {
				if !errors.Is(err, ErrInvalidQuery) {
					t.Fatalf("error = %v, want %v", err, ErrInvalidQuery)
				}
				if repo.lastFind != "" {
					t.Errorf("a refused query still reached the server: %q", repo.lastFind)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if repo.lastFind != "app.users" {
				t.Errorf("lastFind = %q, want app.users", repo.lastFind)
			}
		})
	}
}

// The sort and projection documents go to the server too, so they get the same
// treatment as the filter.
func TestFindChecksEveryDocument(t *testing.T) {
	javaScript := map[string]any{"$where": "this.a > 1"}

	requests := map[string]FindRequest{
		"sort":       {Collection: "users", Sort: javaScript},
		"projection": {Collection: "users", Projection: javaScript},
	}

	for name, request := range requests {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepo()
			if _, err := NewService(repo).Find(t.Context(), "app", request); !errors.Is(err, ErrInvalidQuery) {
				t.Fatalf("%s error = %v, want %v", name, err, ErrInvalidQuery)
			}
			if repo.lastFind != "" {
				t.Errorf("a refused query still reached the server: %q", repo.lastFind)
			}
		})
	}
}

func TestFindValidatesNames(t *testing.T) {
	tests := []struct{ name, database, collection string }{
		{name: "a reserved collection", database: "app", collection: "system.users"},
		{name: "a collection with a dollar sign", database: "app", collection: "user$s"},
		{name: "a database with a dot", database: "a.b", collection: "users"},
		{name: "an empty collection", database: "app", collection: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepo()
			_, err := NewService(repo).Find(t.Context(), test.database,
				FindRequest{Collection: test.collection})
			if !errors.Is(err, ErrInvalidName) {
				t.Fatalf("error = %v, want %v", err, ErrInvalidName)
			}
			if repo.lastFind != "" {
				t.Errorf("a refused query still reached the server: %q", repo.lastFind)
			}
		})
	}
}
