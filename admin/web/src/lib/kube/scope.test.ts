import { describe, expect, it } from "vitest";
import { ALL_NAMESPACES, isAllNamespaces, isNamespaceName, readScope } from "./scope";

describe("readScope", () => {
	it("reads a namespace name as itself", () => {
		expect(readScope("platform-system")).toBe("platform-system");
	});

	it("reads the asterisk as the whole cluster", () => {
		expect(readScope(ALL_NAMESPACES)).toBe(ALL_NAMESPACES);
	});

	// The cookie is whatever a browser sends, and the value ends up in a URL path
	// segment. Falling back to the whole cluster is what keeps a tampered or
	// stale one from becoming a request about a namespace nobody named.
	it.each([
		["absent", undefined],
		["empty", ""],
		["uppercase", "Platform"],
		["a path traversal attempt", "../../secrets"],
		["a name past the label limit", "a".repeat(64)],
	])("reads %s as the whole cluster", (_name, stored) => {
		expect(readScope(stored)).toBe(ALL_NAMESPACES);
	});
});

describe("isAllNamespaces", () => {
	it("is true only for the asterisk", () => {
		expect(isAllNamespaces(ALL_NAMESPACES)).toBe(true);
		expect(isAllNamespaces("admin")).toBe(false);
	});
});

describe("isNamespaceName", () => {
	// The asterisk has to stay outside the set of real names, or a namespace
	// could be created that the panel would read as "all of them".
	it("refuses the all-namespaces marker", () => {
		expect(isNamespaceName(ALL_NAMESPACES)).toBe(false);
	});

	it.each(["admin", "platform-system", "a", "app-1"])("accepts %s", (name) => {
		expect(isNamespaceName(name)).toBe(true);
	});

	it.each(["-leading", "trailing-", "Upper", "with.dot", "with_underscore", ""])(
		"refuses %s",
		(name) => {
			expect(isNamespaceName(name)).toBe(false);
		},
	);
});
