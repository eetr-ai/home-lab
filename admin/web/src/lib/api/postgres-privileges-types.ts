/**
 * The PostgreSQL privilege wire types. Mirrors the privilege types in
 * admin/api/internal/postgres/types.go; kept beside types.ts, which re-exports
 * them, so neither file outgrows the line limit.
 */

/**
 * What one role holds on one object. `granted` is what was granted to the role by
 * name — what a revoke from it removes. `effective` also counts PUBLIC, inherited
 * memberships, ownership and superuser; a privilege only there has to be revoked
 * wherever it was granted.
 */
export interface PostgresObjectPrivileges {
	/** Absent for the database and for a schema itself. */
	schema?: string;
	name: string;
	/** "database", "schema", "table", "view", "matview", "foreign" or "sequence". */
	kind: string;
	owner: string;
	granted: string[];
	/**
	 * Entries of `granted` made by a role other than the owner. The panel's revoke
	 * acts as the owner and cannot remove them, so their boxes are not offered.
	 */
	foreignGrants: { privilege: string; grantor: string }[];
	effective: string[];
}

/**
 * An ALTER DEFAULT PRIVILEGES entry naming the role: objects `owner` creates later
 * get these privileges. Only that owner's objects — not anybody else's.
 */
export interface PostgresDefaultPrivileges {
	owner: string;
	/** Absent when the entry applies in every schema. */
	schema?: string;
	objectType: "tables" | "sequences";
	privileges: string[];
}

/** Everything one role holds in one database. */
export interface PostgresRolePrivileges {
	role: string;
	database: string;
	/** A superuser bypasses every check; its effective lists are always full. */
	isSuperuser: boolean;
	databasePrivileges: PostgresObjectPrivileges;
	schemas: PostgresObjectPrivileges[];
	tables: PostgresObjectPrivileges[];
	sequences: PostgresObjectPrivileges[];
	defaults: PostgresDefaultPrivileges[];
}

export type PostgresPrivilegeTarget =
	| "database"
	| "schema"
	| "table"
	| "sequence"
	| "allTables"
	| "allSequences"
	| "defaultTables"
	| "defaultSequences";

/** One GRANT or REVOKE. Which fields apply depends on the target; see the API. */
export interface PostgresPrivilegeChange {
	action: "grant" | "revoke";
	target: PostgresPrivilegeTarget;
	schema?: string;
	objects?: string[];
	privileges: string[];
	/** The role whose future objects a default target covers. */
	owner?: string;
}

/** What a change ran, and where it left the role. */
export interface ChangePostgresPrivilegesResult {
	statements: string[];
	privileges: PostgresRolePrivileges;
}
