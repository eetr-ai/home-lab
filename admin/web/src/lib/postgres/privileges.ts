import type {
	PostgresObjectPrivileges,
	PostgresPrivilegeChange,
	PostgresRolePrivileges,
} from "@/lib/api/types";

/**
 * The privileges the panel shows, per kind of object, in the order the API
 * returns and accepts them. The API holds the allowlist; this is the same list,
 * so the grid has a column for exactly what can be granted.
 */
export const PRIVILEGES = {
	database: ["CONNECT", "CREATE", "TEMPORARY"],
	schema: ["USAGE", "CREATE"],
	table: ["SELECT", "INSERT", "UPDATE", "DELETE", "TRUNCATE", "REFERENCES", "TRIGGER"],
	sequence: ["USAGE", "SELECT", "UPDATE"],
} as const;

/**
 * The API's bounds on one request: at most this many changes, and at most this
 * many names in one change. diffChanges splits a group at the second; the first is
 * checked before submitting, so a draft over it is explained rather than refused.
 */
export const MAX_CHANGES_PER_REQUEST = 100;
export const MAX_OBJECTS_PER_CHANGE = 200;

/** The four grids, each a kind of object the role can hold privileges on. */
export type PrivilegeSection = keyof typeof PRIVILEGES;

/**
 * The ticked boxes: for every object, the privileges granted to the role by name.
 * Keyed by objectKey, so a table and a sequence of the same name, or two tables
 * of one name in different schemas, never share an entry.
 */
export type PrivilegeDraft = Record<string, string[]>;

/**
 * The key one object's boxes are stored under. JSON rather than a separator,
 * because a catalog name may contain any separator anybody would pick.
 */
export function objectKey(section: PrivilegeSection, object: { schema?: string; name: string }): string {
	return JSON.stringify([section, object.schema ?? "", object.name]);
}

/** Every object of every section, with the section it belongs to. */
function eachObject(privileges: PostgresRolePrivileges): [PrivilegeSection, PostgresObjectPrivileges][] {
	const all: [PrivilegeSection, PostgresObjectPrivileges][] = [["database", privileges.databasePrivileges]];
	for (const object of privileges.schemas) all.push(["schema", object]);
	for (const object of privileges.tables) all.push(["table", object]);
	for (const object of privileges.sequences) all.push(["sequence", object]);
	return all;
}

/** The draft a page starts from: what is granted now. */
export function baselineDraft(privileges: PostgresRolePrivileges): PrivilegeDraft {
	const draft: PrivilegeDraft = {};
	for (const [section, object] of eachObject(privileges)) {
		draft[objectKey(section, object)] = [...object.granted];
	}
	return draft;
}

/** Tick or untick one box, keeping the privileges in their canonical order. */
export function togglePrivilege(
	draft: PrivilegeDraft,
	section: PrivilegeSection,
	object: { schema?: string; name: string },
	privilege: string,
): PrivilegeDraft {
	const key = objectKey(section, object);
	const current = draft[key] ?? [];
	const next = current.includes(privilege)
		? current.filter((one) => one !== privilege)
		: PRIVILEGES[section].filter((one) => one === privilege || current.includes(one));
	return { ...draft, [key]: next };
}

/**
 * The grants and revokes that turn what is granted into what is ticked.
 *
 * Grouped, so ticking SELECT, INSERT, UPDATE and DELETE on two tables is one
 * GRANT naming both — which is the statement anybody would have written by hand,
 * and the one shown back afterwards. Objects are grouped when they share a
 * section, a schema, an action and an identical set of privileges; a database or
 * schema is its own object and never groups.
 *
 * Revokes come before grants. Nothing depends on that order inside one
 * transaction, but it reads in the order a person would do it.
 */
export function diffChanges(
	privileges: PostgresRolePrivileges,
	draft: PrivilegeDraft,
): PostgresPrivilegeChange[] {
	const groups = new Map<string, PostgresPrivilegeChange>();

	for (const [section, object] of eachObject(privileges)) {
		const wanted = draft[objectKey(section, object)] ?? object.granted;
		const added = PRIVILEGES[section].filter((one) => wanted.includes(one) && !object.granted.includes(one));
		const removed = PRIVILEGES[section].filter((one) => !wanted.includes(one) && object.granted.includes(one));

		for (const [action, list] of [["revoke", removed], ["grant", added]] as const) {
			if (list.length === 0) continue;
			const change = changeFor(section, object, action, list);
			if (section === "table" || section === "sequence") {
				// A full group starts a new entry, so no change names more objects
				// than the API accepts in one; the counter keeps the keys distinct.
				const base = JSON.stringify([action, section, object.schema, list]);
				let part = 0;
				while ((groups.get(`${base}#${part}`)?.objects?.length ?? 0) >= MAX_OBJECTS_PER_CHANGE) part += 1;
				const existing = groups.get(`${base}#${part}`);
				if (existing) {
					existing.objects?.push(object.name);
					continue;
				}
				groups.set(`${base}#${part}`, change);
			} else {
				groups.set(JSON.stringify([action, section, object.schema ?? "", object.name]), change);
			}
		}
	}

	const changes = [...groups.values()];
	return [...changes.filter((one) => one.action === "revoke"), ...changes.filter((one) => one.action === "grant")];
}

function changeFor(
	section: PrivilegeSection,
	object: PostgresObjectPrivileges,
	action: "grant" | "revoke",
	privileges: string[],
): PostgresPrivilegeChange {
	switch (section) {
		case "database":
			return { action, target: "database", privileges };
		case "schema":
			return { action, target: "schema", schema: object.name, privileges };
		default:
			return { action, target: section, schema: object.schema, objects: [object.name], privileges };
	}
}

/** How many boxes differ from what is granted — the count on the save button. */
export function countDifferences(privileges: PostgresRolePrivileges, draft: PrivilegeDraft): number {
	let count = 0;
	for (const [section, object] of eachObject(privileges)) {
		const wanted = draft[objectKey(section, object)] ?? object.granted;
		for (const privilege of PRIVILEGES[section]) {
			if (wanted.includes(privilege) !== object.granted.includes(privilege)) count += 1;
		}
	}
	return count;
}

export type SchemaPreset = "readOnly" | "readWrite";

export const PRESET_LABELS: Record<SchemaPreset, string> = {
	readOnly: "Read-only",
	readWrite: "Read-write",
};

/**
 * The changes behind a preset: everything an application role needs to use a
 * schema, applied together so it lands whole.
 *
 * - CONNECT on the database. Usually PUBLIC already has it, but a lab that
 *   revoked it from PUBLIC is exactly where a role would otherwise fail to log in.
 * - USAGE on the schema, without which no privilege on a table inside it works.
 * - The table privileges on every table that exists in the schema now.
 * - On every sequence: SELECT to read one, and for read-write USAGE as well, which
 *   is what nextval — a serial column's default — needs. Forgetting it is the
 *   classic "INSERT fails on a table I can INSERT into".
 *
 * `futureOwner`, when set, adds the same table and sequence privileges as default
 * privileges for objects that role creates later in the schema. Only that role's:
 * a table created by anybody else is not covered.
 */
export function presetChanges(
	preset: SchemaPreset,
	schema: string,
	futureOwner: string | null,
): PostgresPrivilegeChange[] {
	const tables = preset === "readOnly" ? ["SELECT"] : ["SELECT", "INSERT", "UPDATE", "DELETE"];
	const sequences = preset === "readOnly" ? ["SELECT"] : ["USAGE", "SELECT"];

	const changes: PostgresPrivilegeChange[] = [
		{ action: "grant", target: "database", privileges: ["CONNECT"] },
		{ action: "grant", target: "schema", schema, privileges: ["USAGE"] },
		{ action: "grant", target: "allTables", schema, privileges: tables },
		{ action: "grant", target: "allSequences", schema, privileges: sequences },
	];
	if (futureOwner) {
		changes.push(
			{ action: "grant", target: "defaultTables", schema, owner: futureOwner, privileges: tables },
			{ action: "grant", target: "defaultSequences", schema, owner: futureOwner, privileges: sequences },
		);
	}
	return changes;
}

/**
 * Whether an object's boxes are offered at all. An owner is granted everything by
 * default and revoking its own rights is a foot-gun nobody reaches for from a
 * grid; a superuser bypasses every check.
 */
export function isEditable(privileges: PostgresRolePrivileges, object: PostgresObjectPrivileges): boolean {
	return !privileges.isSuperuser && object.owner !== privileges.role;
}

/**
 * Who granted a privilege, when it was not the object's owner — in which case
 * the box is locked. The panel's revoke acts as the owner and would leave that
 * grant in place (the API refuses it rather than report success), so it has to
 * be revoked by whoever granted it.
 */
export function foreignGrantor(object: PostgresObjectPrivileges, privilege: string): string | undefined {
	return object.foreignGrants.find((grant) => grant.privilege === privilege)?.grantor;
}

/**
 * The part of the loaded state the grid is drawn from — everything but the
 * default privileges. The grid's reset identity is built from this, so revoking a
 * default-privileges entry does not throw away boxes ticked but not yet applied.
 */
export function gridIdentity(privileges: PostgresRolePrivileges): string {
	// An undefined value is left out by JSON.stringify, so this is the state
	// without its defaults.
	return JSON.stringify({ ...privileges, defaults: undefined });
}
