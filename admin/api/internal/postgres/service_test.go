package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeRepo stands in for PostgreSQL. It records what it was asked to do, so a
// test can assert that a refusal happened *before* anything reached the server
// rather than being reported after the fact.
type fakeRepo struct {
	databases  map[string]string // name -> owner
	roles      map[string]bool   // name -> canLogin
	extensions map[string][]Extension
	current    string

	created []string
	dropped []string
	err     error

	// What the service actually asked for, so a test can check the request was
	// carried through rather than only that a call happened.
	lastDatabaseOwner string
	lastRoleRequest   CreateRoleRequest
	lastExtension     string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		databases:  map[string]string{"postgres": "admin", "template0": "admin", "template1": "admin"},
		roles:      map[string]bool{"admin": true, "reporting": false},
		extensions: map[string][]Extension{},
		current:    "admin",
	}
}

func (f *fakeRepo) ListDatabases(context.Context) ([]Database, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := make([]Database, 0, len(f.databases))
	for name, owner := range f.databases {
		out = append(out, Database{Name: name, Owner: owner})
	}
	return out, nil
}

func (f *fakeRepo) CreateDatabase(_ context.Context, name, owner string) error {
	f.created = append(f.created, "database:"+name)
	f.lastDatabaseOwner = owner
	f.databases[name] = owner
	return f.err
}

func (f *fakeRepo) DropDatabase(_ context.Context, name string) error {
	f.dropped = append(f.dropped, "database:"+name)
	delete(f.databases, name)
	return f.err
}

func (f *fakeRepo) DatabaseExists(_ context.Context, name string) (bool, error) {
	_, ok := f.databases[name]
	return ok, f.err
}

func (f *fakeRepo) ListRoles(context.Context) ([]Role, error) {
	out := make([]Role, 0, len(f.roles))
	for name, canLogin := range f.roles {
		out = append(out, Role{Name: name, CanLogin: canLogin})
	}
	return out, f.err
}

func (f *fakeRepo) CreateRole(_ context.Context, req CreateRoleRequest) error {
	f.created = append(f.created, "role:"+req.Name)
	f.lastRoleRequest = req
	f.roles[req.Name] = req.CanLogin
	return f.err
}

func (f *fakeRepo) DropRole(_ context.Context, name string) error {
	f.dropped = append(f.dropped, "role:"+name)
	delete(f.roles, name)
	return f.err
}

func (f *fakeRepo) RoleExists(_ context.Context, name string) (bool, error) {
	_, ok := f.roles[name]
	return ok, f.err
}

func (f *fakeRepo) CurrentRole(context.Context) (string, error) {
	return f.current, f.err
}

func (f *fakeRepo) ListExtensions(_ context.Context, database string) ([]Extension, error) {
	return f.extensions[database], f.err
}

func (f *fakeRepo) CreateExtension(_ context.Context, database, name string) error {
	f.created = append(f.created, "extension:"+database+"."+name)
	f.lastExtension = name
	f.extensions[database] = append(f.extensions[database], Extension{Name: name})
	return f.err
}

const goodPassword = "a-long-enough-password"

func TestCreateDatabase(t *testing.T) {
	tests := []struct {
		name    string
		request CreateDatabaseRequest
		wantErr error
	}{
		{name: "a plain database", request: CreateDatabaseRequest{Name: "app"}},
		{name: "owned by an existing role", request: CreateDatabaseRequest{Name: "app", Owner: "reporting"}},
		{name: "a name with a semicolon", request: CreateDatabaseRequest{Name: "app; DROP DATABASE postgres"}, wantErr: ErrInvalidName},
		{name: "an empty name", request: CreateDatabaseRequest{Name: ""}, wantErr: ErrInvalidName},
		// pg_ is PostgreSQL's own prefix. The server does not always refuse it,
		// and what happens when it does not is confusing enough to refuse here.
		{name: "a reserved prefix", request: CreateDatabaseRequest{Name: "pg_shadowy"}, wantErr: ErrInvalidName},
		{name: "a reserved prefix in mixed case", request: CreateDatabaseRequest{Name: "PG_Shadowy"}, wantErr: ErrInvalidName},
		{name: "an owner that does not exist", request: CreateDatabaseRequest{Name: "app", Owner: "nobody"}, wantErr: ErrNotFound},
		{name: "an owner with an unsafe name", request: CreateDatabaseRequest{Name: "app", Owner: `x" --`}, wantErr: ErrInvalidName},
		{name: "a name already taken", request: CreateDatabaseRequest{Name: "postgres"}, wantErr: ErrAlreadyExists},
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
				// A refusal must not have reached the server first.
				if len(repo.created) != 0 {
					t.Errorf("CreateDatabase() was refused but still called the repository: %v", repo.created)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateDatabase() error = %v", err)
			}
			if len(repo.created) != 1 {
				t.Errorf("CreateDatabase() calls = %v, want exactly one", repo.created)
			}
			if repo.lastDatabaseOwner != test.request.Owner {
				t.Errorf("CreateDatabase() owner = %q, want %q", repo.lastDatabaseOwner, test.request.Owner)
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
		// Dropping template0 or template1 breaks database creation for the whole
		// server; dropping postgres removes the database the panel connects to.
		{name: "postgres itself", database: "postgres", wantErr: ErrProtected},
		{name: "template0", database: "template0", wantErr: ErrProtected},
		{name: "template1", database: "template1", wantErr: ErrProtected},
		{name: "a protected name in mixed case", database: "Template1", wantErr: ErrProtected},
		{name: "one that does not exist", database: "missing", wantErr: ErrNotFound},
		{name: "an unsafe name", database: `app"; DROP`, wantErr: ErrInvalidName},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.databases["app"] = "admin"
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

func TestCreateRole(t *testing.T) {
	tests := []struct {
		name    string
		request CreateRoleRequest
		wantErr error
	}{
		{name: "a login role", request: CreateRoleRequest{Name: "app", CanLogin: true, Password: goodPassword}},
		{name: "a group role", request: CreateRoleRequest{Name: "readers"}},
		{name: "an unsafe name", request: CreateRoleRequest{Name: "app; DROP ROLE admin"}, wantErr: ErrInvalidName},
		{name: "a reserved prefix", request: CreateRoleRequest{Name: "pg_signal_backend"}, wantErr: ErrInvalidName},
		// These roles sit on an unencrypted LAN port with no client allowlist, so
		// the password is the entire access boundary.
		{name: "a login role with a short password", request: CreateRoleRequest{Name: "app", CanLogin: true, Password: "short"}, wantErr: ErrWeakPassword},
		{name: "a login role with no password", request: CreateRoleRequest{Name: "app", CanLogin: true}, wantErr: ErrWeakPassword},
		// Refused rather than ignored: discarding it silently leaves the caller
		// believing a password was set.
		{name: "a group role given a password", request: CreateRoleRequest{Name: "readers", Password: goodPassword}, wantErr: ErrInvalidName},
		{name: "a name already taken", request: CreateRoleRequest{Name: "admin", CanLogin: true, Password: goodPassword}, wantErr: ErrAlreadyExists},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepo()
			service := NewService(repo)

			_, err := service.CreateRole(t.Context(), test.request)

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("CreateRole() error = %v, want %v", err, test.wantErr)
				}
				if len(repo.created) != 0 {
					t.Errorf("CreateRole() was refused but still called the repository")
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateRole() error = %v", err)
			}
			if repo.lastRoleRequest.Name != test.request.Name ||
				repo.lastRoleRequest.CanLogin != test.request.CanLogin ||
				repo.lastRoleRequest.CanCreateDatabase != test.request.CanCreateDatabase ||
				repo.lastRoleRequest.CanCreateRole != test.request.CanCreateRole {
				t.Errorf("CreateRole() passed %+v, want %+v", repo.lastRoleRequest, test.request)
			}
		})
	}
}

// The panel must not drop the account it connects as. PostgreSQL refuses once the
// role owns something, but not reliably, and an admin panel that locked itself out
// of the server it administers is bad enough to check for rather than hope about.
func TestDropRoleRefusesTheConnectingAccount(t *testing.T) {
	for _, name := range []string{"admin", "ADMIN", "Admin"} {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.current = "admin"
			repo.roles[name] = true
			service := NewService(repo)

			err := service.DropRole(t.Context(), name)
			if !errors.Is(err, ErrProtected) {
				t.Fatalf("DropRole(%q) error = %v, want %v", name, err, ErrProtected)
			}
			if len(repo.dropped) != 0 {
				t.Errorf("DropRole(%q) was refused but still called the repository", name)
			}
		})
	}
}

func TestDropRole(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		wantErr error
	}{
		{name: "an ordinary role", role: "reporting"},
		{name: "one of PostgreSQL's own", role: "pg_read_all_data", wantErr: ErrProtected},
		{name: "one that does not exist", role: "missing", wantErr: ErrNotFound},
		{name: "an unsafe name", role: `r"; DROP`, wantErr: ErrInvalidName},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepo()
			service := NewService(repo)

			err := service.DropRole(t.Context(), test.role)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("DropRole(%q) error = %v, want %v", test.role, err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("DropRole(%q) error = %v", test.role, err)
			}
		})
	}
}

func TestCreateExtension(t *testing.T) {
	tests := []struct {
		name      string
		database  string
		extension string
		wantErr   error
	}{
		{name: "pgvector into an existing database", database: "app", extension: "vector"},
		{name: "an extension whose name has a hyphen", database: "app", extension: "uuid-ossp"},
		{name: "into a database that does not exist", database: "missing", extension: "vector", wantErr: ErrNotFound},
		{name: "an unsafe extension name", database: "app", extension: `x"; DROP`, wantErr: ErrInvalidName},
		{name: "an unsafe database name", database: `app"`, extension: "vector", wantErr: ErrInvalidName},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.databases["app"] = "admin"
			service := NewService(repo)

			_, err := service.CreateExtension(t.Context(), test.database, CreateExtensionRequest{Name: test.extension})
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("CreateExtension() error = %v, want %v", err, test.wantErr)
				}
				if len(repo.created) != 0 {
					t.Errorf("CreateExtension() was refused but still called the repository")
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateExtension() error = %v", err)
			}
			if repo.lastExtension != test.extension {
				t.Errorf("CreateExtension() passed %q, want %q", repo.lastExtension, test.extension)
			}
		})
	}
}

// Creating a database installs no extension. Extensions are per database, so a
// default would mean every database carried pgvector whether or not anything used
// it — and the caller could not tell which had been chosen for it.
func TestCreateDatabaseInstallsNoExtensions(t *testing.T) {
	repo := newFakeRepo()
	service := NewService(repo)

	if _, err := service.CreateDatabase(t.Context(), CreateDatabaseRequest{Name: "app"}); err != nil {
		t.Fatalf("CreateDatabase() error = %v", err)
	}
	for _, call := range repo.created {
		if strings.HasPrefix(call, "extension:") {
			t.Errorf("CreateDatabase() installed %s", call)
		}
	}
}
