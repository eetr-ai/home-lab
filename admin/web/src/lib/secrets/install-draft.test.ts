import { describe, expect, it } from "vitest";
import { EMPTY_INSTALL, planInstall, secretName } from "./install-draft";

const credential = { username: "octo", password: "hunter2hunter2hu" };
const on = { ...EMPTY_INSTALL, enabled: true, namespace: "octo" };

describe("planInstall", () => {
	it("plans nothing when the section is switched off", () => {
		expect(planInstall(EMPTY_INSTALL, credential)).toBeNull();
	});

	it("plans the write when the section is filled in", () => {
		expect(planInstall({ ...on, name: "octo-database" }, credential)).toEqual({
			ok: true,
			namespace: "octo",
			name: "octo-database",
			request: { data: { username: "octo", password: "hunter2hunter2hu" }, overwrite: false },
		});
	});

	it("names the Secret after the credential when nothing was typed", () => {
		const plan = planInstall(on, credential);
		expect(plan?.ok && plan.name).toBe("octo-credentials");
	});

	it("needs a namespace", () => {
		const plan = planInstall({ ...on, namespace: "" }, credential);
		expect(plan).toEqual({ ok: false, error: "Choose a namespace for the Secret." });
	});

	// The layout's own rules still apply once the section is on, and this is the
	// path they reach the operator through.
	it("carries a bad layout's reason through", () => {
		const plan = planInstall({ ...on, usernameKey: "password" }, credential);
		expect(plan).toEqual({ ok: false, error: 'Two fields are both named "password".' });
	});
});

describe("secretName", () => {
	it("prefers what the operator typed, trimmed", () => {
		expect(secretName({ ...on, name: "  octo-database " }, "octo")).toBe("octo-database");
	});

	it("is empty when there is nothing to name it after", () => {
		expect(secretName(on, "")).toBe("");
	});
});
