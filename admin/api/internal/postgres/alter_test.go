package postgres

import (
	"strings"
	"testing"
)

// ALTER ROLE leaves an unmentioned attribute alone, so every one has to be
// written in both directions. Omitting the negative form would silently fail to
// revoke a privilege — and the panel would then display a role as no longer
// having it while it still did.
func TestAlterRoleStatementWritesBothDirections(t *testing.T) {
	tests := []struct {
		name     string
		request  UpdateRoleRequest
		contains []string
	}{
		{
			name:     "everything granted",
			request:  UpdateRoleRequest{CanLogin: true, CanCreateDatabase: true, CanCreateRole: true, ConnectionLimit: -1},
			contains: []string{" LOGIN", " CREATEDB", " CREATEROLE", "CONNECTION LIMIT -1"},
		},
		{
			name:     "everything revoked",
			request:  UpdateRoleRequest{ConnectionLimit: 5},
			contains: []string{" NOLOGIN", " NOCREATEDB", " NOCREATEROLE", "CONNECTION LIMIT 5"},
		},
		{
			// Superuser is never granted here, on any path.
			name:     "superuser is always refused",
			request:  UpdateRoleRequest{CanLogin: true, ConnectionLimit: -1},
			contains: []string{"NOSUPERUSER"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			statement, err := alterRoleStatement("app", test.request)
			if err != nil {
				t.Fatalf("alterRoleStatement() error = %v", err)
			}
			for _, want := range test.contains {
				if !strings.Contains(statement, want) {
					t.Errorf("statement %q does not contain %q", statement, want)
				}
			}
		})
	}
}

// The plaintext password must never appear in a statement: it would be visible in
// pg_stat_activity while the statement ran, and written to the server log
// whenever log_statement is on — neither of which this panel controls.
func TestAlterRoleStatementSendsAVerifierNotThePassword(t *testing.T) {
	const password = "correct horse battery staple"

	statement, err := alterRoleStatement("app", UpdateRoleRequest{
		CanLogin: true, ConnectionLimit: -1, Password: password,
	})
	if err != nil {
		t.Fatalf("alterRoleStatement() error = %v", err)
	}

	if strings.Contains(statement, password) {
		t.Fatal("the statement carries the plaintext password")
	}
	if !strings.Contains(statement, "SCRAM-SHA-256$") {
		t.Errorf("the statement carries no SCRAM verifier: %q", statement)
	}
}

// An empty password leaves the existing one alone. Writing PASSWORD NULL would
// remove it, which is not what an edit form that could not show it meant.
func TestAlterRoleStatementLeavesAnUnsetPasswordAlone(t *testing.T) {
	statement, err := alterRoleStatement("app", UpdateRoleRequest{CanLogin: true, ConnectionLimit: -1})
	if err != nil {
		t.Fatalf("alterRoleStatement() error = %v", err)
	}
	if strings.Contains(statement, "PASSWORD") {
		t.Errorf("statement %q touches the password", statement)
	}
}

// Every identifier goes through quoteIdentifier. This is the assertion that the
// alter path is not the one that forgot.
func TestAlterRoleStatementRefusesAnUnsafeName(t *testing.T) {
	names := []string{`app"; DROP ROLE other; --`, "app role", "1app", strings.Repeat("a", 64), ""}

	for _, name := range names {
		if _, err := alterRoleStatement(name, UpdateRoleRequest{ConnectionLimit: -1}); err == nil {
			t.Errorf("alterRoleStatement(%q) was accepted", name)
		}
	}
}
