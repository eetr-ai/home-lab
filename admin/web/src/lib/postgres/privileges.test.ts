import { describe, expect, it } from "vitest";
import type { PostgresObjectPrivileges, PostgresRolePrivileges } from "@/lib/api/types";
import {
	baselineDraft,
	countDifferences,
	diffChanges,
	MAX_OBJECTS_PER_CHANGE,
	foreignGrantor,
	gridIdentity,
	isEditable,
	objectKey,
	presetChanges,
	togglePrivilege,
} from "./privileges";

function table(name: string, granted: string[] = [], schema = "public"): PostgresObjectPrivileges {
	return { schema, name, kind: "table", owner: "migrator", granted, foreignGrants: [], effective: granted };
}

function state(overrides: Partial<PostgresRolePrivileges> = {}): PostgresRolePrivileges {
	return {
		role: "app",
		database: "appdb",
		isSuperuser: false,
		databasePrivileges: { name: "appdb", kind: "database", owner: "migrator", granted: [], foreignGrants: [], effective: ["CONNECT"] },
		schemas: [{ name: "public", kind: "schema", owner: "migrator", granted: [], foreignGrants: [], effective: ["USAGE"] }],
		tables: [table("products"), table("packages"), table("secrets", ["SELECT"])],
		sequences: [
			{ schema: "public", name: "products_id_seq", kind: "sequence", owner: "migrator", granted: [], foreignGrants: [], effective: [] },
			{ schema: "public", name: "packages_id_seq", kind: "sequence", owner: "migrator", granted: [], foreignGrants: [], effective: [] },
		],
		defaults: [],
		...overrides,
	};
}

describe("diffChanges", () => {
	it("is empty when nothing was ticked or unticked", () => {
		const privileges = state();
		expect(diffChanges(privileges, baselineDraft(privileges))).toEqual([]);
	});

	// The grant that motivated the feature, built by ticking boxes: it has to come
	// out as the two statements somebody would have typed, not as twelve.
	it("groups the same privileges on several objects into one change", () => {
		const privileges = state();
		let draft = baselineDraft(privileges);
		for (const name of ["products", "packages"]) {
			for (const privilege of ["DELETE", "SELECT", "UPDATE", "INSERT"]) {
				draft = togglePrivilege(draft, "table", { schema: "public", name }, privilege);
			}
		}
		for (const name of ["products_id_seq", "packages_id_seq"]) {
			draft = togglePrivilege(draft, "sequence", { schema: "public", name }, "USAGE");
		}

		expect(diffChanges(privileges, draft)).toEqual([
			{
				action: "grant",
				target: "table",
				schema: "public",
				objects: ["products", "packages"],
				privileges: ["SELECT", "INSERT", "UPDATE", "DELETE"],
			},
			{
				action: "grant",
				target: "sequence",
				schema: "public",
				objects: ["products_id_seq", "packages_id_seq"],
				privileges: ["USAGE"],
			},
		]);
	});

	it("does not group objects whose privilege sets differ, or that sit in different schemas", () => {
		const privileges = state({ tables: [table("a"), table("b"), table("c", [], "other")] });
		let draft = baselineDraft(privileges);
		draft = togglePrivilege(draft, "table", { schema: "public", name: "a" }, "SELECT");
		draft = togglePrivilege(draft, "table", { schema: "public", name: "b" }, "SELECT");
		draft = togglePrivilege(draft, "table", { schema: "public", name: "b" }, "INSERT");
		draft = togglePrivilege(draft, "table", { schema: "other", name: "c" }, "SELECT");

		const changes = diffChanges(privileges, draft);
		expect(changes.map((one) => [one.schema, one.objects, one.privileges])).toEqual([
			["public", ["a"], ["SELECT"]],
			["public", ["b"], ["SELECT", "INSERT"]],
			["other", ["c"], ["SELECT"]],
		]);
	});

	// Only what was granted by name can be revoked. Unticking it revokes it; an
	// untouched box with an inherited privilege must not turn into anything.
	it("revokes what was unticked, before any grant", () => {
		const privileges = state();
		let draft = baselineDraft(privileges);
		draft = togglePrivilege(draft, "table", { schema: "public", name: "products" }, "SELECT");
		draft = togglePrivilege(draft, "table", { schema: "public", name: "secrets" }, "SELECT");

		expect(diffChanges(privileges, draft)).toEqual([
			{ action: "revoke", target: "table", schema: "public", objects: ["secrets"], privileges: ["SELECT"] },
			{ action: "grant", target: "table", schema: "public", objects: ["products"], privileges: ["SELECT"] },
		]);
	});

	it("addresses the database and a schema by their own targets", () => {
		const privileges = state();
		let draft = baselineDraft(privileges);
		draft = togglePrivilege(draft, "database", { name: "appdb" }, "CONNECT");
		draft = togglePrivilege(draft, "schema", { name: "public" }, "USAGE");

		expect(diffChanges(privileges, draft)).toEqual([
			{ action: "grant", target: "database", privileges: ["CONNECT"] },
			{ action: "grant", target: "schema", schema: "public", privileges: ["USAGE"] },
		]);
	});

	it("ticking and unticking the same box is no change", () => {
		const privileges = state();
		const object = { schema: "public", name: "products" };
		const draft = togglePrivilege(togglePrivilege(baselineDraft(privileges), "table", object, "SELECT"), "table", object, "SELECT");
		expect(diffChanges(privileges, draft)).toEqual([]);
		expect(countDifferences(privileges, draft)).toBe(0);
	});
});

describe("togglePrivilege", () => {
	it("keeps the canonical order whatever order the boxes were ticked in", () => {
		const object = { schema: "public", name: "t" };
		let draft = togglePrivilege({}, "table", object, "DELETE");
		draft = togglePrivilege(draft, "table", object, "SELECT");
		expect(draft[objectKey("table", object)]).toEqual(["SELECT", "DELETE"]);
	});

	it("keeps a table and a sequence of the same name apart", () => {
		const object = { schema: "public", name: "same" };
		const draft = togglePrivilege({}, "table", object, "SELECT");
		expect(draft[objectKey("sequence", object)]).toBeUndefined();
	});
});

describe("countDifferences", () => {
	it("counts boxes, not objects", () => {
		const privileges = state();
		let draft = baselineDraft(privileges);
		draft = togglePrivilege(draft, "table", { schema: "public", name: "products" }, "SELECT");
		draft = togglePrivilege(draft, "table", { schema: "public", name: "products" }, "INSERT");
		draft = togglePrivilege(draft, "table", { schema: "public", name: "secrets" }, "SELECT");
		expect(countDifferences(privileges, draft)).toBe(3);
	});
});

describe("presetChanges", () => {
	// Read-write without USAGE on sequences is the classic "INSERT fails on a
	// table I can INSERT into": a serial column's default calls nextval.
	it("read-write grants nextval on sequences along with row access", () => {
		const changes = presetChanges("readWrite", "public", null);
		expect(changes).toEqual([
			{ action: "grant", target: "database", privileges: ["CONNECT"] },
			{ action: "grant", target: "schema", schema: "public", privileges: ["USAGE"] },
			{ action: "grant", target: "allTables", schema: "public", privileges: ["SELECT", "INSERT", "UPDATE", "DELETE"] },
			{ action: "grant", target: "allSequences", schema: "public", privileges: ["USAGE", "SELECT"] },
		]);
	});

	// USAGE on a sequence is nextval, which advances it — a write.
	it("read-only grants nothing that writes", () => {
		const changes = presetChanges("readOnly", "public", null);
		const onTables = changes.filter((one) => one.target === "allTables").flatMap((one) => one.privileges);
		const onSequences = changes.filter((one) => one.target === "allSequences").flatMap((one) => one.privileges);
		expect(onTables).toEqual(["SELECT"]);
		expect(onSequences).toEqual(["SELECT"]);
	});

	it("covers future objects only for the owner it is given", () => {
		const changes = presetChanges("readOnly", "public", "migrator");
		const defaults = changes.filter((one) => one.target.startsWith("default"));
		expect(defaults).toEqual([
			{ action: "grant", target: "defaultTables", schema: "public", owner: "migrator", privileges: ["SELECT"] },
			{ action: "grant", target: "defaultSequences", schema: "public", owner: "migrator", privileges: ["SELECT"] },
		]);
	});
});

describe("isEditable", () => {
	it("refuses what the role owns, and everything for a superuser", () => {
		const privileges = state();
		expect(isEditable(privileges, table("t"))).toBe(true);
		expect(isEditable(privileges, { ...table("t"), owner: "app" })).toBe(false);
		expect(isEditable({ ...privileges, isSuperuser: true }, table("t"))).toBe(false);
	});
});

describe("diffChanges limits", () => {
	// The API takes at most 200 names in one change; a bigger group is split into
	// several changes in the same request rather than refused whole.
	it("splits a group larger than the per-change limit", () => {
		const tables = Array.from({ length: MAX_OBJECTS_PER_CHANGE + 1 }, (_, i) => table(`t${i}`));
		const privileges = state({ tables });
		let draft = baselineDraft(privileges);
		for (const one of tables) draft = togglePrivilege(draft, "table", one, "SELECT");

		const changes = diffChanges(privileges, draft);
		expect(changes.map((one) => one.objects?.length)).toEqual([MAX_OBJECTS_PER_CHANGE, 1]);
		expect(changes.flatMap((one) => one.objects)).toEqual(tables.map((one) => one.name));
	});
});

describe("foreignGrantor", () => {
	it("names who granted a privilege when it was not the owner", () => {
		const object = { ...table("t", ["SELECT"]), foreignGrants: [{ privilege: "SELECT", grantor: "lead" }] };
		expect(foreignGrantor(object, "SELECT")).toBe("lead");
		expect(foreignGrantor(object, "INSERT")).toBeUndefined();
	});
});

describe("gridIdentity", () => {
	// Revoking a default-privileges entry must not reset boxes ticked but not yet
	// applied, so the defaults are not part of what the grid is keyed by.
	it("ignores default privileges and follows everything else", () => {
		const before = state();
		const defaultsChanged = { ...before, defaults: [{ owner: "m", objectType: "tables" as const, privileges: ["SELECT"] }] };
		expect(gridIdentity(defaultsChanged)).toBe(gridIdentity(before));
		expect(gridIdentity({ ...before, tables: [table("products", ["SELECT"])] })).not.toBe(gridIdentity(before));
	});
});
