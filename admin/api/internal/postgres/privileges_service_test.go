package postgres

import (
	"context"
	"errors"
	"testing"
)

func selectOn(table string) []PrivilegeChange {
	return []PrivilegeChange{{Action: "grant", Target: "table", Schema: "public", Objects: []string{table}, Privileges: []string{"SELECT"}}}
}

func TestChangePrivileges(t *testing.T) {
	tests := []struct {
		name     string
		database string
		role     string
		changes  []PrivilegeChange
		wantErr  error
	}{
		{name: "a grant to an application role", database: "appdb", role: "reporting", changes: selectOn("t")},
		{name: "a malformed change", database: "appdb", role: "reporting", changes: []PrivilegeChange{{Action: "grant", Target: "table"}}, wantErr: ErrInvalidName},
		{name: "a database that does not exist", database: "nowhere", role: "reporting", changes: selectOn("t"), wantErr: ErrNotFound},
		{name: "a role that does not exist", database: "appdb", role: "nobody", changes: selectOn("t"), wantErr: ErrNotFound},
		// The panel's own account, and every other superuser: a grant to either
		// succeeds and changes nothing, so it is refused rather than shown done.
		{name: "the connecting account", database: "appdb", role: "admin", changes: selectOn("t"), wantErr: ErrProtected},
		{name: "the connecting account in another case", database: "appdb", role: "ADMIN", changes: selectOn("t"), wantErr: ErrProtected},
		{name: "another superuser", database: "appdb", role: "root", changes: selectOn("t"), wantErr: ErrProtected},
		{name: "a predefined role", database: "appdb", role: "pg_read_all_data", changes: selectOn("t"), wantErr: ErrProtected},
		// template1 is copied into every new database, so a grant there spreads.
		{name: "a template database", database: "template1", role: "reporting", changes: selectOn("t"), wantErr: ErrProtected},
		{name: "the administrative database", database: "postgres", role: "reporting", changes: selectOn("t"), wantErr: ErrProtected},
		{
			name: "a default owner that does not exist", database: "appdb", role: "reporting",
			changes: []PrivilegeChange{{Action: "grant", Target: "defaultTables", Owner: "ghost", Privileges: []string{"SELECT"}}},
			wantErr: ErrNotFound,
		},
		{
			name: "a default owner that exists", database: "appdb", role: "reporting",
			changes: []PrivilegeChange{{Action: "grant", Target: "defaultTables", Owner: "migrator", Privileges: []string{"SELECT"}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.databases["appdb"] = "migrator"
			repo.roles["migrator"] = true
			repo.roles["root"] = true
			repo.superusers["root"] = true
			service := NewService(repo)

			result, err := service.ChangePrivileges(t.Context(), test.database, test.role, ChangePrivilegesRequest{Changes: test.changes})

			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("ChangePrivileges() error = %v, want %v", err, test.wantErr)
				}
				// A refusal must not have reached the server first.
				if len(repo.appliedStatements) != 0 {
					t.Errorf("ChangePrivileges() was refused but still ran %q", repo.appliedStatements)
				}
				return
			}
			if err != nil {
				t.Fatalf("ChangePrivileges() error = %v", err)
			}
			if repo.appliedDatabase != test.database {
				t.Errorf("statements ran in %q, want %q", repo.appliedDatabase, test.database)
			}
			// What is reported as run is exactly what was run.
			if len(result.Statements) != len(test.changes) || len(repo.appliedStatements) != len(result.Statements) ||
				result.Statements[0] != repo.appliedStatements[0] {
				t.Errorf("reported %q, ran %q", result.Statements, repo.appliedStatements)
			}
			if result.Privileges.Role != test.role || result.Privileges.Database != test.database {
				t.Errorf("returned the state of %s in %s, want %s in %s",
					result.Privileges.Role, result.Privileges.Database, test.role, test.database)
			}
		})
	}
}

// A server-side failure is passed back as it came, so a 422 still carries
// PostgreSQL's own message naming the object it could not find.
func TestChangePrivilegesPassesTheServerFailureThrough(t *testing.T) {
	repo := newFakeRepo()
	repo.databases["appdb"] = "admin"
	service := NewService(&failingApply{fakeRepo: repo, err: ErrQueryFailed})

	_, err := service.ChangePrivileges(t.Context(), "appdb", "reporting", ChangePrivilegesRequest{Changes: selectOn("gone")})
	if !errors.Is(err, ErrQueryFailed) {
		t.Fatalf("ChangePrivileges() error = %v, want ErrQueryFailed", err)
	}
}

// failingApply fails only the apply, so the checks before it pass.
type failingApply struct {
	*fakeRepo
	err error
}

func (f *failingApply) ApplyPrivileges(_ context.Context, _ string, _ []string) error {
	return f.err
}

func TestListPrivileges(t *testing.T) {
	tests := []struct {
		name          string
		database      string
		role          string
		wantErr       error
		wantSuperuser bool
	}{
		{name: "an application role", database: "appdb", role: "reporting"},
		// Listing a superuser is allowed — it is changing one that is not — and
		// says so, because its every effective list is full.
		{name: "a superuser is marked", database: "appdb", role: "admin", wantSuperuser: true},
		{name: "an unsafe database", database: "app db", role: "reporting", wantErr: ErrInvalidName},
		{name: "an unsafe role", database: "appdb", role: "x;y", wantErr: ErrInvalidName},
		{name: "a missing database", database: "nowhere", role: "reporting", wantErr: ErrNotFound},
		{name: "a missing role", database: "appdb", role: "nobody", wantErr: ErrNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepo()
			repo.databases["appdb"] = "admin"
			service := NewService(repo)

			got, err := service.ListPrivileges(t.Context(), test.database, test.role)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("ListPrivileges() error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ListPrivileges() error = %v", err)
			}
			if got.IsSuperuser != test.wantSuperuser {
				t.Errorf("IsSuperuser = %v, want %v", got.IsSuperuser, test.wantSuperuser)
			}
		})
	}
}
