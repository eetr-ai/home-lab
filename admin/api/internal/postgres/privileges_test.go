package postgres

import (
	"errors"
	"strings"
	"testing"
)

// The statements a change produces. These are the exact strings sent to the
// server, so each case is a statement somebody could paste into psql — and the
// first is the grant that motivated the feature.
func TestPrivilegeStatements(t *testing.T) {
	tests := []struct {
		name   string
		change PrivilegeChange
		want   string
	}{
		{
			name: "named tables",
			change: PrivilegeChange{
				Action: "grant", Target: "table", Schema: "public",
				Objects: []string{"products", "packages"}, Privileges: []string{"SELECT", "INSERT", "UPDATE", "DELETE"},
			},
			want: `GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE "public"."products", "public"."packages" TO "app"`,
		},
		{
			name: "named sequences",
			change: PrivilegeChange{
				Action: "grant", Target: "sequence", Schema: "public",
				Objects: []string{"products_id_seq"}, Privileges: []string{"USAGE"},
			},
			want: `GRANT USAGE ON SEQUENCE "public"."products_id_seq" TO "app"`,
		},
		{
			name:   "a revoke says FROM",
			change: PrivilegeChange{Action: "revoke", Target: "table", Schema: "public", Objects: []string{"t"}, Privileges: []string{"DELETE"}},
			want:   `REVOKE DELETE ON TABLE "public"."t" FROM "app"`,
		},
		{
			name:   "the database comes from the path",
			change: PrivilegeChange{Action: "grant", Target: "database", Privileges: []string{"CONNECT", "TEMPORARY"}},
			want:   `GRANT CONNECT, TEMPORARY ON DATABASE "appdb" TO "app"`,
		},
		{
			name:   "a schema",
			change: PrivilegeChange{Action: "grant", Target: "schema", Schema: "public", Privileges: []string{"USAGE"}},
			want:   `GRANT USAGE ON SCHEMA "public" TO "app"`,
		},
		{
			name:   "every table in a schema",
			change: PrivilegeChange{Action: "grant", Target: "allTables", Schema: "public", Privileges: []string{"SELECT"}},
			want:   `GRANT SELECT ON ALL TABLES IN SCHEMA "public" TO "app"`,
		},
		{
			name:   "every sequence in a schema",
			change: PrivilegeChange{Action: "revoke", Target: "allSequences", Schema: "public", Privileges: []string{"USAGE"}},
			want:   `REVOKE USAGE ON ALL SEQUENCES IN SCHEMA "public" FROM "app"`,
		},
		{
			// FOR ROLE is always written: left out, the default would cover only
			// the tables the panel's own account creates.
			name: "future tables of one owner in one schema",
			change: PrivilegeChange{
				Action: "grant", Target: "defaultTables", Schema: "public", Owner: "migrator",
				Privileges: []string{"SELECT"},
			},
			want: `ALTER DEFAULT PRIVILEGES FOR ROLE "migrator" IN SCHEMA "public" GRANT SELECT ON TABLES TO "app"`,
		},
		{
			name:   "future sequences of one owner in every schema",
			change: PrivilegeChange{Action: "revoke", Target: "defaultSequences", Owner: "migrator", Privileges: []string{"USAGE"}},
			want:   `ALTER DEFAULT PRIVILEGES FOR ROLE "migrator" REVOKE USAGE ON SEQUENCES FROM "app"`,
		},
		{
			// Lower case, duplicated and out of order in; one canonical statement
			// out, so the same request always runs the same SQL.
			name:   "privileges are canonicalised",
			change: PrivilegeChange{Action: "grant", Target: "table", Schema: "public", Objects: []string{"t"}, Privileges: []string{"delete", "select", "SELECT"}},
			want:   `GRANT SELECT, DELETE ON TABLE "public"."t" TO "app"`,
		},
		{
			// Catalog names are escaped, not allowlisted: a real table may be
			// capitalised or contain a quote, and must still be grantable.
			name:   "an awkward table name is escaped",
			change: PrivilegeChange{Action: "grant", Target: "table", Schema: "Sales", Objects: []string{`odd"name`}, Privileges: []string{"SELECT"}},
			want:   `GRANT SELECT ON TABLE "Sales"."odd""name" TO "app"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := privilegeStatements("appdb", "app", []PrivilegeChange{test.change})
			if err != nil {
				t.Fatalf("privilegeStatements() error = %v", err)
			}
			if len(got) != 1 || got[0] != test.want {
				t.Errorf("privilegeStatements() = %q, want %q", got, test.want)
			}
		})
	}
}

// Every keyword that reaches a statement comes from an allowlist, and every field
// a target does not read is refused rather than ignored.
func TestPrivilegeStatementsRefusals(t *testing.T) {
	table := func(mutate func(*PrivilegeChange)) PrivilegeChange {
		change := PrivilegeChange{Action: "grant", Target: "table", Schema: "public", Objects: []string{"t"}, Privileges: []string{"SELECT"}}
		mutate(&change)
		return change
	}

	tests := []struct {
		name     string
		database string
		role     string
		changes  []PrivilegeChange
	}{
		{name: "no changes", changes: nil},
		{name: "too many changes", changes: make([]PrivilegeChange, maxPrivilegeChanges+1)},
		{name: "an unsafe role", role: `app" --`, changes: []PrivilegeChange{table(func(*PrivilegeChange) {})}},
		{name: "an unsafe database", database: "app;db", changes: []PrivilegeChange{table(func(*PrivilegeChange) {})}},
		{name: "an unknown action", changes: []PrivilegeChange{table(func(c *PrivilegeChange) { c.Action = "give" })}},
		{name: "an unknown target", changes: []PrivilegeChange{table(func(c *PrivilegeChange) { c.Target = "function" })}},
		// The injection attempt the allowlist exists for.
		{name: "a privilege carrying SQL", changes: []PrivilegeChange{table(func(c *PrivilegeChange) { c.Privileges = []string{"SELECT ON t TO x; DROP TABLE t; --"} })}},
		{name: "ALL is not offered", changes: []PrivilegeChange{table(func(c *PrivilegeChange) { c.Privileges = []string{"ALL"} })}},
		{name: "a privilege of another kind", changes: []PrivilegeChange{table(func(c *PrivilegeChange) { c.Privileges = []string{"USAGE"} })}},
		{name: "no privileges", changes: []PrivilegeChange{table(func(c *PrivilegeChange) { c.Privileges = nil })}},
		{name: "a table with no schema", changes: []PrivilegeChange{table(func(c *PrivilegeChange) { c.Schema = "" })}},
		{name: "a table with no objects", changes: []PrivilegeChange{table(func(c *PrivilegeChange) { c.Objects = nil })}},
		{name: "a null byte in an object", changes: []PrivilegeChange{table(func(c *PrivilegeChange) { c.Objects = []string{"t\x00"} })}},
		{name: "objects with allTables", changes: []PrivilegeChange{table(func(c *PrivilegeChange) { c.Target = "allTables" })}},
		{name: "an owner on a table", changes: []PrivilegeChange{table(func(c *PrivilegeChange) { c.Owner = "migrator" })}},
		{
			name:    "a schema on the database",
			changes: []PrivilegeChange{{Action: "grant", Target: "database", Schema: "public", Privileges: []string{"CONNECT"}}},
		},
		{
			name:    "a default with no owner",
			changes: []PrivilegeChange{{Action: "grant", Target: "defaultTables", Schema: "public", Privileges: []string{"SELECT"}}},
		},
		{
			name:    "a default with an unsafe owner",
			changes: []PrivilegeChange{{Action: "grant", Target: "defaultTables", Owner: `x" --`, Privileges: []string{"SELECT"}}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database, role := test.database, test.role
			if database == "" {
				database = "appdb"
			}
			if role == "" {
				role = "app"
			}
			got, err := privilegeStatements(database, role, test.changes)
			if !errors.Is(err, ErrInvalidName) {
				t.Fatalf("privilegeStatements() = %q, %v; want ErrInvalidName", got, err)
			}
		})
	}
}

// A refusal in a batch names which change it was; "change 2" is something an
// operator can act on, "an unknown privilege" among twelve is not.
func TestPrivilegeStatementsNumberTheFailingChange(t *testing.T) {
	_, err := privilegeStatements("appdb", "app", []PrivilegeChange{
		{Action: "grant", Target: "database", Privileges: []string{"CONNECT"}},
		{Action: "grant", Target: "schema", Schema: "public", Privileges: []string{"DROP"}},
	})
	if err == nil || !strings.Contains(err.Error(), "change 2") {
		t.Fatalf("privilegeStatements() error = %v, want it to name change 2", err)
	}
}

// A listing reports privileges in the allowlist's order and drops any the panel
// does not show (MAINTAIN, on PostgreSQL 17 and later), so it agrees with what
// the panel will accept.
func TestOrderPrivileges(t *testing.T) {
	got := orderPrivileges(tablePrivileges, []string{"DELETE", "MAINTAIN", "SELECT"})
	if strings.Join(got, ",") != "SELECT,DELETE" {
		t.Errorf("orderPrivileges() = %v, want [SELECT DELETE]", got)
	}
	if got := orderPrivileges(tablePrivileges, nil); got == nil || len(got) != 0 {
		// An empty list, not null: the panel iterates it.
		t.Errorf("orderPrivileges(nil) = %#v, want an empty slice", got)
	}
}
