import { describe, expect, it } from "vitest";
import { searchHref, searchKey } from "./search";

describe("searchHref", () => {
	it("searching the same pattern twice gives two addresses, so the second starts over", () => {
		expect(searchHref("lock:*", 1)).not.toBe(searchHref("lock:*", 2));
	});

	it("an empty pattern leaves the parameter out rather than setting it empty", () => {
		expect(searchHref("", 5)).toBe("?t=5");
	});

	it("keeps leading and trailing spaces, which Redis key names may have", () => {
		expect(new URLSearchParams(searchHref(" orders:* ", 1)).get("pattern")).toBe(" orders:* ");
	});

	it("encodes the pattern as a query value", () => {
		expect(searchHref("a&b", 1)).toBe("?pattern=a%26b&t=1");
	});
});

describe("searchKey", () => {
	it("is stable for one address, so a revalidation keeps the browser's place", () => {
		expect(searchKey("lock:*", "1")).toBe(searchKey("lock:*", "1"));
	});

	it("cannot confuse a pattern with the timestamp beside it", () => {
		expect(searchKey("a1", "")).not.toBe(searchKey("a", "1"));
	});
});
