import { describe, expect, it } from "vitest";
import { filterRanked, rankSearchString } from "./rank";

describe("rankSearchString", () => {
	it("scores a prefix as 1 and a query with an absent letter below a real match", () => {
		expect(rankSearchString("user", "users")).toBe(1);
		// "xyz" shares nothing meaningful; it stays well under the filter threshold.
		expect(rankSearchString("xyz", "users")).toBeLessThan(0.4);
	});

	it("rewards a tight match over a spread-out one", () => {
		// "ord" is contiguous in "orders" but spread across "on_road".
		expect(rankSearchString("ord", "orders")).toBeGreaterThan(rankSearchString("ord", "on_road"));
	});

	it("is case-insensitive", () => {
		expect(rankSearchString("USERS", "users")).toBe(1);
	});
});

describe("filterRanked", () => {
	const tables = ["agent_threads", "agent_turns", "users", "user_sessions", "orders"];

	it("returns everything in original order for an empty query", () => {
		expect(filterRanked(tables, "", (t) => t)).toEqual(tables);
		expect(filterRanked(tables, "   ", (t) => t)).toEqual(tables);
	});

	it("ranks the closest match first and drops the unrelated", () => {
		const result = filterRanked(tables, "user", (t) => t);
		expect(result[0]).toBe("users");
		expect(result).toContain("user_sessions");
		expect(result).not.toContain("orders");
	});

	it("keeps equal scores in the caller's order", () => {
		// Same prefix, same length — a genuine tie under the unbiased scorer, so the
		// input order is what decides, and reversing the input reverses the output.
		const items = ["cat_alpha", "cat_omega"];
		expect(filterRanked(items, "cat", (t) => t, { bias: "unbiased" })).toEqual(items);
		expect(filterRanked([...items].reverse(), "cat", (t) => t, { bias: "unbiased" })).toEqual([
			"cat_omega",
			"cat_alpha",
		]);
	});

	it("honours the limit", () => {
		expect(filterRanked(tables, "", (t) => t, { limit: 2 })).toEqual(tables.slice(0, 2));
	});
});
