import { describe, expect, it } from "vitest";
import { describeUser, effectivePrefix } from "./users";

describe("effectivePrefix", () => {
	it("defaults to the name and a colon", () => {
		expect(effectivePrefix("orders", "")).toBe("orders:");
	});

	it("keeps a prefix the operator typed, trimmed", () => {
		expect(effectivePrefix("orders", "  shop:orders:  ")).toBe("shop:orders:");
	});

	it("is empty when there is no name to default from, so the API's refusal is what the operator sees", () => {
		expect(effectivePrefix("", "")).toBe("");
	});
});

describe("describeUser", () => {
	it("names the preset and the prefix for a user the panel wrote", () => {
		expect(
			describeUser({ name: "orders", enabled: true, hasPassword: true, prefix: "orders:", preset: "readonly", editable: true }),
		).toBe("Read only on orders:*");
	});

	it("says a user has no password rather than calling its rules custom", () => {
		expect(
			describeUser({ name: "legacy", enabled: true, hasPassword: false, preset: "custom", editable: false }),
		).toBe("No password, set outside the panel");
	});

	it("does not dress up a hand-made user as the nearest preset", () => {
		expect(
			describeUser({ name: "legacy", enabled: true, hasPassword: true, preset: "custom", editable: false }),
		).toBe("Custom rules, set outside the panel");
	});
});
