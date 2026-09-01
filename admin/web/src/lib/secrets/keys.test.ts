import { describe, expect, it } from "vitest";
import { isValidSecretKey } from "./keys";

describe("isValidSecretKey", () => {
	it("accepts the shapes charts actually use", () => {
		for (const key of [
			"password",
			"username",
			"POSTGRES_PASSWORD",
			"ca.crt",
			"tls.key",
			"redis-password",
			"connection_string",
			"a",
			"0",
			"a".repeat(253),
		]) {
			expect(isValidSecretKey(key), key).toBe(true);
		}
	});

	it("refuses what Kubernetes would refuse", () => {
		for (const key of [
			"", // an unfilled row, which is the common way to arrive here
			"pass word",
			"pass/word", // a slash would be a second path segment on a mounted volume
			"pass:word",
			"café",
			"a".repeat(254),
		]) {
			expect(isValidSecretKey(key), JSON.stringify(key)).toBe(false);
		}
	});
});
