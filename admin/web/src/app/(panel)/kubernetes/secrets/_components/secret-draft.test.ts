import { describe, expect, it } from "vitest";
import { planCreate, planRotate, type SecretRow } from "./secret-draft";
import type { SecretSummary } from "@/lib/api/types";

function rows(...pairs: [string, string][]): SecretRow[] {
	return pairs.map(([key, value], index) => ({ id: String(index), key, value }));
}

const secret: SecretSummary = {
	name: "octo-database",
	type: "Opaque",
	keys: ["database", "password", "username"],
	immutable: false,
	panelManaged: true,
	removable: true,
	createdAt: "2026-08-01T00:00:00Z",
};

describe("planCreate", () => {
	it("turns filled rows into a payload", () => {
		const plan = planCreate({
			name: "octo-database",
			rows: rows(["username", "octo"], ["password", "hunter2"]),
			overwrite: false,
		});
		expect(plan).toEqual({
			ok: true,
			name: "octo-database",
			request: { data: { username: "octo", password: "hunter2" }, overwrite: false },
		});
	});

	// The mistake this exists to catch. Object.fromEntries keeps the last of two
	// identical keys, so this would write a Secret whose username key holds the
	// password — and nothing reads a value back, so the first sign of it would be
	// a workload that will not authenticate.
	it("refuses two rows with the same key", () => {
		const plan = planCreate({
			name: "octo-database",
			rows: rows(["password", "one"], ["password", "two"]),
			overwrite: false,
		});
		expect(plan.ok).toBe(false);
		if (!plan.ok) expect(plan.error).toContain("password");
	});

	// A row nobody touched is not an error — the form starts with blank rows and
	// offers more than are usually wanted.
	it("ignores rows that are entirely empty", () => {
		const plan = planCreate({
			name: "octo-database",
			rows: rows(["password", "hunter2"], ["", ""], ["", ""]),
			overwrite: false,
		});
		expect(plan.ok).toBe(true);
		if (plan.ok) expect(plan.request.data).toEqual({ password: "hunter2" });
	});

	// Half-filled is different from empty: one of them is a mistake in progress.
	it("refuses a half-filled row", () => {
		expect(planCreate({ name: "s", rows: rows(["password", ""]), overwrite: false }).ok).toBe(false);
		expect(planCreate({ name: "s", rows: rows(["", "hunter2"]), overwrite: false }).ok).toBe(false);
	});

	it("refuses a Secret with no keys at all", () => {
		expect(planCreate({ name: "octo-database", rows: rows(["", ""]), overwrite: false }).ok).toBe(
			false,
		);
	});

	it("refuses a key Kubernetes would not accept", () => {
		const plan = planCreate({
			name: "octo-database",
			rows: rows(["pass word", "hunter2"]),
			overwrite: false,
		});
		expect(plan.ok).toBe(false);
		if (!plan.ok) expect(plan.error).toContain("pass word");
	});

	it("refuses a name the API would refuse", () => {
		for (const name of ["", "  ", "Octo", "octo_database", "octo.database", "-octo", "octo-"]) {
			expect(planCreate({ name, rows: rows(["k", "v"]), overwrite: false }).ok, name).toBe(false);
		}
	});

	it("trims the name rather than sending the spaces", () => {
		const plan = planCreate({
			name: "  octo-database  ",
			rows: rows(["password", "hunter2"]),
			overwrite: false,
		});
		expect(plan.ok).toBe(true);
		if (plan.ok) expect(plan.name).toBe("octo-database");
	});

	it("carries overwrite through", () => {
		const plan = planCreate({
			name: "octo-database",
			rows: rows(["password", "hunter2"]),
			overwrite: true,
		});
		expect(plan.ok).toBe(true);
		if (plan.ok) expect(plan.request.overwrite).toBe(true);
	});
});

describe("planRotate", () => {
	it("sends only the keys being rotated", () => {
		const plan = planRotate(secret, rows(["password", "new-password"]));
		expect(plan).toEqual({ ok: true, request: { data: { password: "new-password" } } });
	});

	// The rule that keeps rotation from being a create with weaker guards.
	it("refuses a key the Secret does not have", () => {
		const plan = planRotate(secret, rows(["apikey", "new-value"]));
		expect(plan.ok).toBe(false);
		if (!plan.ok) expect(plan.error).toContain("apikey");
	});

	it("refuses a rotation that would change nothing", () => {
		expect(planRotate(secret, []).ok).toBe(false);
		expect(planRotate(secret, rows(["", ""])).ok).toBe(false);
	});

	it("refuses an empty new value", () => {
		expect(planRotate(secret, rows(["password", ""])).ok).toBe(false);
	});
});
