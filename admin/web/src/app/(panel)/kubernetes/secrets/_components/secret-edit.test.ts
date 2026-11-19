import { describe, expect, it } from "vitest";
import {
	emptyEdit,
	isEditDirty,
	planEdit,
	withAction,
	withReplacedValues,
	type EditDraft,
} from "./secret-edit";
import type { SecretRow } from "./secret-draft";
import type { SecretSummary } from "@/lib/api/types";

const secret: SecretSummary = {
	name: "octo-database",
	type: "Opaque",
	keys: ["database", "password", "username"],
	immutable: false,
	panelManaged: true,
	removable: true,
	createdAt: "2026-08-01T00:00:00Z",
};

function added(...pairs: [string, string][]): SecretRow[] {
	return pairs.map(([key, value], index) => ({ id: String(index), key, value, format: "password" }));
}

function replacing(draft: EditDraft, key: string, value: string): EditDraft {
	const next = withAction(draft, key, "replace");
	return { ...next, existing: { ...next.existing, [key]: { ...next.existing[key], value } } };
}

describe("planEdit", () => {
	it("sends each key in the list that says what is happening to it", () => {
		let draft = replacing(emptyEdit(), "password", "new-password");
		draft = withAction(draft, "database", "remove");
		draft = { ...draft, added: added(["apikey", "new-key"]) };

		expect(planEdit(secret, draft)).toEqual({
			ok: true,
			request: {
				replace: { password: "new-password" },
				add: { apikey: "new-key" },
				remove: ["database"],
			},
		});
	});

	it("leaves out the lists with nothing in them", () => {
		const plan = planEdit(secret, withAction(emptyEdit(), "database", "remove"));
		expect(plan).toEqual({ ok: true, request: { remove: ["database"] } });
	});

	it("sends a multi-line value exactly as typed", () => {
		const pem = "-----BEGIN PRIVATE KEY-----\nMIIBVQ==\n-----END PRIVATE KEY-----\n";
		const plan = planEdit(secret, replacing(emptyEdit(), "password", pem));
		expect(plan.ok && plan.request.replace?.password).toBe(pem);
	});

	it("refuses an edit that changes nothing", () => {
		expect(planEdit(secret, emptyEdit()).ok).toBe(false);
		// A blank added row is not a change.
		expect(planEdit(secret, { ...emptyEdit(), added: added(["", ""]) }).ok).toBe(false);
	});

	// Dropping it as unfilled would let the operator believe the key was rotated.
	it("refuses a key marked for replacement with no new value", () => {
		const plan = planEdit(secret, withAction(emptyEdit(), "password", "replace"));
		expect(plan.ok).toBe(false);
		if (!plan.ok) expect(plan.error).toContain("password");
	});

	// Adding is not a quiet overwrite, and the API would refuse it.
	it("refuses adding a key the Secret already has", () => {
		const plan = planEdit(secret, { ...emptyEdit(), added: added(["password", "x"]) });
		expect(plan.ok).toBe(false);
		if (!plan.ok) expect(plan.error).toContain("Replace");
	});

	it("refuses removing a key and adding it back under the same name", () => {
		const draft = withAction(emptyEdit(), "password", "remove");
		const plan = planEdit(secret, { ...draft, added: added(["password", "x"]) });
		expect(plan.ok).toBe(false);
	});

	it("refuses two added rows with the same key", () => {
		const plan = planEdit(secret, { ...emptyEdit(), added: added(["apikey", "a"], ["apikey", "b"]) });
		expect(plan.ok).toBe(false);
	});

	it("refuses an added key Kubernetes would not accept", () => {
		const plan = planEdit(secret, { ...emptyEdit(), added: added(["api key", "a"]) });
		expect(plan.ok).toBe(false);
	});

	it("refuses removing every key, which is a delete", () => {
		let draft = emptyEdit();
		for (const key of secret.keys) draft = withAction(draft, key, "remove");
		const plan = planEdit(secret, draft);
		expect(plan.ok).toBe(false);
		if (!plan.ok) expect(plan.error).toContain("Delete");
	});

	it("allows removing every key when another is added in its place", () => {
		let draft = emptyEdit();
		for (const key of secret.keys) draft = withAction(draft, key, "remove");
		const plan = planEdit(secret, { ...draft, added: added(["apikey", "a"]) });
		expect(plan.ok).toBe(true);
	});
});

describe("withAction", () => {
	// Switching a key away and back must not resurrect a value typed earlier —
	// the field would reappear holding a credential nobody is looking at.
	it("resets the value when the action changes", () => {
		let draft = replacing(emptyEdit(), "password", "typed");
		draft = withAction(draft, "password", "keep");
		draft = withAction(draft, "password", "replace");
		expect(draft.existing.password.value).toBe("");
	});

	it("forgets a key put back to keep", () => {
		const draft = withAction(withAction(emptyEdit(), "password", "remove"), "password", "keep");
		expect(draft.existing).toEqual({});
	});
});

// A key is whatever the Secret holds. Assigning "__proto__" through a plain
// object sets its prototype instead, and the next copy loses the entry.
describe("a key named __proto__", () => {
	const odd: SecretSummary = { ...secret, keys: ["__proto__", "password"] };

	it("survives being replaced and copied", () => {
		let draft = withAction(emptyEdit(), "__proto__", "replace");
		draft = withReplacedValues(draft, [
			{ id: "__proto__", key: "__proto__", value: "new", format: "password" },
		]);
		draft = { ...draft, existing: { ...draft.existing } };

		expect(isEditDirty(draft)).toBe(true);
		const plan = planEdit(odd, draft);
		expect(plan.ok && Object.keys(plan.request.replace ?? {})).toEqual(["__proto__"]);
	});

	// Acting on a second key copies the map, which is where an entry stored as
	// the prototype rather than as an own property would be lost.
	it("survives another key being acted on after it", () => {
		let draft = withAction(emptyEdit(), "__proto__", "remove");
		draft = withAction(draft, "password", "remove");
		expect(planEdit(odd, { ...draft, added: [{ id: "0", key: "apikey", value: "x", format: "password" }] }))
			.toEqual({ ok: true, request: { add: { apikey: "x" }, remove: ["__proto__", "password"] } });
	});
});

describe("isEditDirty", () => {
	it("is clean with nothing chosen and only blank added rows", () => {
		expect(isEditDirty(emptyEdit())).toBe(false);
		expect(isEditDirty({ ...emptyEdit(), added: added(["", ""]) })).toBe(false);
	});

	it("is dirty once a key is acted on or a row is typed into", () => {
		expect(isEditDirty(withAction(emptyEdit(), "password", "remove"))).toBe(true);
		expect(isEditDirty({ ...emptyEdit(), added: added(["apikey", ""]) })).toBe(true);
	});
});
