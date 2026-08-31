import { describe, expect, it } from "vitest";
import { credentialSecretData, DEFAULT_LAYOUT, type SecretLayout } from "./db-secret";

const credential = { username: "octo", password: "hunter2hunter2hu", database: "octo" };

describe("credentialSecretData", () => {
	it("carries the credential under the keys the chart expects", () => {
		const result = credentialSecretData(credential, DEFAULT_LAYOUT);
		expect(result).toEqual({ ok: true, data: { username: "octo", password: "hunter2hunter2hu" } });
	});

	it("includes the database only when a key is named for it", () => {
		const result = credentialSecretData(credential, { ...DEFAULT_LAYOUT, databaseKey: "dbname" });
		expect(result.ok && result.data.dbname).toBe("octo");
	});

	it("omits the database when the credential has none", () => {
		const result = credentialSecretData(
			{ username: "octo", password: "hunter2hunter2hu" },
			{ ...DEFAULT_LAYOUT, databaseKey: "dbname" },
		);
		expect(result.ok && "dbname" in result.data).toBe(false);
	});

	it("drops the username when its key is cleared", () => {
		const result = credentialSecretData(credential, { ...DEFAULT_LAYOUT, usernameKey: "" });
		expect(result).toEqual({ ok: true, data: { password: "hunter2hunter2hu" } });
	});

	// The one that matters. Two keys with the same name leave one value in an
	// object literal, and the survivor is whichever was written last — a Secret
	// whose password key holds the username, with nothing to say so.
	it("refuses two fields under one key", () => {
		const layout: SecretLayout = { usernameKey: "password", passwordKey: "password", databaseKey: "" };
		expect(credentialSecretData(credential, layout)).toEqual({
			ok: false,
			error: 'Two fields are both named "password".',
		});
	});

	it("refuses a Secret with nowhere to put the password", () => {
		const result = credentialSecretData(credential, { ...DEFAULT_LAYOUT, passwordKey: "" });
		expect(result.ok).toBe(false);
	});

	it("refuses a key Kubernetes would not accept", () => {
		const result = credentialSecretData(credential, { ...DEFAULT_LAYOUT, passwordKey: "pass word" });
		expect(result).toEqual({ ok: false, error: '"pass word" is not a valid Secret key.' });
	});

	// A role created without a password cannot go into a Secret: the value would
	// be empty, and an empty password key is a workload that starts and fails to
	// authenticate.
	it("refuses an empty value", () => {
		const result = credentialSecretData({ username: "octo", password: "" }, DEFAULT_LAYOUT);
		expect(result).toEqual({ ok: false, error: '"password" would have no value.' });
	});
});
