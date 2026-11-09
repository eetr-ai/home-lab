import { describe, expect, it } from "vitest";
import {
	canUseFormat,
	keepLineEndings,
	pasteKeepingLineEndings,
	shownValue,
	usesCrlf,
	planCreate,
	planRotate,
	type CreateDraft,
	type SecretRow,
} from "./secret-draft";
import type { SecretSummary } from "@/lib/api/types";

function rows(...pairs: [string, string][]): SecretRow[] {
	return pairs.map(([key, value], index) => ({ id: String(index), key, value, format: "password" }));
}

/** A draft with a namespace already chosen, for the cases that are about the rest. */
function draft(partial: Omit<CreateDraft, "namespace">): CreateDraft {
	return { namespace: "apps", ...partial };
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
		const plan = planCreate(draft({
			name: "octo-database",
			rows: rows(["username", "octo"], ["password", "hunter2"]),
			overwrite: false,
		}));
		expect(plan).toEqual({
			ok: true,
			namespace: "apps",
			name: "octo-database",
			request: { data: { username: "octo", password: "hunter2" }, overwrite: false },
		});
	});

	// The mistake that started this: the namespace used to come from the page, and
	// the page used to fall back to whichever namespace sorts first. A Secret in
	// the wrong namespace is a real Secret that nothing complains about — the
	// release that needed it simply never finds it — so a draft without one is
	// refused here rather than written somewhere plausible.
	it("refuses a draft with no namespace", () => {
		const plan = planCreate({
			namespace: "",
			name: "octo-database",
			rows: rows(["password", "hunter2"]),
			overwrite: false,
		});
		expect(plan.ok).toBe(false);
		if (!plan.ok) expect(plan.error).toContain("namespace");
	});

	// The mistake this exists to catch. Object.fromEntries keeps the last of two
	// identical keys, so this would write a Secret whose username key holds the
	// password — and nothing reads a value back, so the first sign of it would be
	// a workload that will not authenticate.
	it("refuses two rows with the same key", () => {
		const plan = planCreate(draft({
			name: "octo-database",
			rows: rows(["password", "one"], ["password", "two"]),
			overwrite: false,
		}));
		expect(plan.ok).toBe(false);
		if (!plan.ok) expect(plan.error).toContain("password");
	});

	// A row nobody touched is not an error — the form starts with blank rows and
	// offers more than are usually wanted.
	it("ignores rows that are entirely empty", () => {
		const plan = planCreate(draft({
			name: "octo-database",
			rows: rows(["password", "hunter2"], ["", ""], ["", ""]),
			overwrite: false,
		}));
		expect(plan.ok).toBe(true);
		if (plan.ok) expect(plan.request.data).toEqual({ password: "hunter2" });
	});

	// Half-filled is different from empty: one of them is a mistake in progress.
	it("refuses a half-filled row", () => {
		expect(planCreate(draft({ name: "s", rows: rows(["password", ""]), overwrite: false })).ok).toBe(false);
		expect(planCreate(draft({ name: "s", rows: rows(["", "hunter2"]), overwrite: false })).ok).toBe(false);
	});

	it("refuses a Secret with no keys at all", () => {
		expect(
			planCreate(draft({ name: "octo-database", rows: rows(["", ""]), overwrite: false })).ok,
		).toBe(false);
	});

	it("refuses a key Kubernetes would not accept", () => {
		const plan = planCreate(draft({
			name: "octo-database",
			rows: rows(["pass word", "hunter2"]),
			overwrite: false,
		}));
		expect(plan.ok).toBe(false);
		if (!plan.ok) expect(plan.error).toContain("pass word");
	});

	it("refuses a name the API would refuse", () => {
		for (const name of ["", "  ", "Octo", "octo_database", "octo.database", "-octo", "octo-"]) {
			expect(planCreate(draft({ name, rows: rows(["k", "v"]), overwrite: false })).ok, name).toBe(
				false,
			);
		}
	});

	it("trims the name rather than sending the spaces", () => {
		const plan = planCreate(draft({
			name: "  octo-database  ",
			rows: rows(["password", "hunter2"]),
			overwrite: false,
		}));
		expect(plan.ok).toBe(true);
		if (plan.ok) expect(plan.name).toBe("octo-database");
	});

	it("carries overwrite through", () => {
		const plan = planCreate(draft({
			name: "octo-database",
			rows: rows(["password", "hunter2"]),
			overwrite: true,
		}));
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

describe("file values", () => {
	const pem = "-----BEGIN PRIVATE KEY-----\nMIIBVQ==\n-----END PRIVATE KEY-----\n";

	// The bug that asked for the textarea: line breaks are the value. Nothing
	// between the form and the payload may trim or join them, trailing one included.
	it("sends a multi-line value exactly as typed", () => {
		const plan = planCreate(draft({
			name: "tls",
			rows: [{ id: "0", key: "tls.key", value: pem, format: "file" }],
			overwrite: false,
		}));
		expect(plan.ok && plan.request.data["tls.key"]).toBe(pem);
	});

	it("rotates to a multi-line value exactly as typed", () => {
		const plan = planRotate(secret, [{ id: "0", key: "password", value: pem, format: "file" }]);
		expect(plan.ok && plan.request.data.password).toBe(pem);
	});
});

describe("canUseFormat", () => {
	it("always allows a file", () => {
		expect(canUseFormat({ value: "a\nb" }, "file")).toBe(true);
	});

	it("allows a password for a single-line value", () => {
		expect(canUseFormat({ value: "hunter2" }, "password")).toBe(true);
		expect(canUseFormat({ value: "" }, "password")).toBe(true);
	});

	// A password input shows the value with its breaks removed, and the next
	// keystroke writes that back — so the switch is refused instead.
	it("refuses a password once the value has a line break", () => {
		expect(canUseFormat({ value: "a\nb" }, "password")).toBe(false);
		expect(canUseFormat({ value: "a\r\nb" }, "password")).toBe(false);
	});
});

describe("line endings", () => {
	const crlf = "-----BEGIN CERTIFICATE-----\r\nMIIB\r\n-----END CERTIFICATE-----\r\n";

	it("knows an all-CRLF value from an LF or mixed one", () => {
		expect(usesCrlf(crlf)).toBe(true);
		expect(usesCrlf("a\nb")).toBe(false);
		expect(usesCrlf("a\r\nb\nc")).toBe(false);
		expect(usesCrlf("a\rb\r\n")).toBe(false);
		expect(usesCrlf("one line")).toBe(false);
	});

	it("shows the textarea LF", () => {
		expect(shownValue(crlf)).toBe(crlf.replace(/\r\n/g, "\n"));
	});

	// The finding: a CRLF paste reached the payload as LF. Pasted into an empty
	// field it must arrive byte for byte, and survive an edit after it.
	it("keeps a CRLF paste, and the CRLF through an edit", () => {
		const pasted = pasteKeepingLineEndings("", 0, 0, crlf);
		expect(pasted).toBe(crlf);

		const typed = shownValue(crlf).replace("MIIB", "MIIBVQ");
		const edited = keepLineEndings(crlf, typed);
		expect(edited).toBe(crlf.replace("MIIB", "MIIBVQ"));

		const plan = planCreate(draft({
			name: "tls",
			rows: [{ id: "0", key: "tls.crt", value: edited, format: "file" }],
			overwrite: false,
		}));
		expect(plan.ok && plan.request.data["tls.crt"]).toBe(edited);
	});

	it("splices a CRLF paste at the selection in what the textarea shows", () => {
		// "a\r\nb" is shown as "a\nb"; selecting the "b" (2..3) and pasting.
		expect(pasteKeepingLineEndings("a\r\nb", 2, 3, "x\r\ny")).toBe("a\r\nx\r\ny");
	});

	it("leaves LF values and LF pastes to the browser", () => {
		expect(pasteKeepingLineEndings("", 0, 0, "a\nb")).toBeNull();
		expect(pasteKeepingLineEndings("a\nb", 0, 0, "x\r\ny")).toBeNull();
		expect(keepLineEndings("a\nb", "a\nbc")).toBe("a\nbc");
	});
});
