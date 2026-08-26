package mongo

import (
	"context"
	"errors"
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
	lastReq CreateUserRequest
	err     error
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
