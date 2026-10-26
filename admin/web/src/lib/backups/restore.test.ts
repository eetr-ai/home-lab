import { describe, expect, it } from "vitest";
import type { BackupFile } from "@/lib/api/types";
import { defaultTarget, restoreRequest, sourceOf } from "./restore";

function file(database: string): BackupFile {
	return { id: "f", name: "x.dump", database, size: 1, createdAt: "2026-10-26T03:00:00Z" };
}

describe("sourceOf", () => {
	it("reads the engine and database from the tag", () => {
		expect(sourceOf(file("postgres/app"))).toEqual({ engine: "postgres", database: "app" });
		expect(sourceOf(file("mongo/events"))).toEqual({ engine: "mongo", database: "events" });
	});

	it("offers no restore for the roles file or anything untagged", () => {
		expect(sourceOf(file("postgres/@globals"))).toBeNull();
		expect(sourceOf(file(""))).toBeNull();
		expect(sourceOf(file("mysql/app"))).toBeNull();
	});
});

describe("defaultTarget", () => {
	it("names the copy after the database and the day, in UTC", () => {
		expect(defaultTarget("app", new Date("2026-10-26T23:30:00-03:00"))).toBe("app_restored_20261027");
	});
});

describe("restoreRequest", () => {
	it("restores into a new database by name", () => {
		expect(restoreRequest("app", "new", " app_copy ", "")).toEqual({
			ok: true,
			request: { mode: "new", target: "app_copy" },
		});
	});

	// Replace drops the live database: the name typed back, exactly, or nothing.
	it("replaces only with the name typed back exactly", () => {
		expect(restoreRequest("app", "replace", "", "app")).toEqual({
			ok: true,
			request: { mode: "replace", confirm: "app" },
		});
		expect(restoreRequest("app", "replace", "", "App").ok).toBe(false);
		expect(restoreRequest("app", "replace", "", "").ok).toBe(false);
	});

	it("refuses a new database named after the source, or a name the API would", () => {
		expect(restoreRequest("app", "new", "app", "").ok).toBe(false);
		expect(restoreRequest("app", "new", "x'; drop", "").ok).toBe(false);
		expect(restoreRequest("app", "new", "", "").ok).toBe(false);
	});
});
