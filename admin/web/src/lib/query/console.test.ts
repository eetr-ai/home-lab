import { describe, expect, it } from "vitest";
import { describeResult, parseDocument } from "./console";

describe("parseDocument", () => {
	it("treats a blank field as an empty document", () => {
		// "Show me everything" is the commonest thing to ask. Requiring `{}` would
		// be a rule with nothing behind it.
		expect(parseDocument("", "filter")).toEqual({ document: {} });
		expect(parseDocument("   \n ", "filter")).toEqual({ document: {} });
	});

	it("parses an object", () => {
		expect(parseDocument('{"status": "active"}', "filter")).toEqual({
			document: { status: "active" },
		});
	});

	it("parses a nested document", () => {
		expect(parseDocument('{"count": {"$gt": 5}}', "filter")).toEqual({
			document: { count: { $gt: 5 } },
		});
	});

	const refused: { name: string; raw: string }[] = [
		{ name: "malformed JSON", raw: "{status: active}" },
		// Both of these are valid JSON, and both would reach the server and come
		// back with a message that says less than this one.
		{ name: "an array", raw: "[1, 2]" },
		{ name: "null", raw: "null" },
		{ name: "a bare string", raw: '"abc"' },
		{ name: "a number", raw: "42" },
	];

	for (const test of refused) {
		it(`refuses ${test.name}`, () => {
			const got = parseDocument(test.raw, "filter");
			expect("error" in got).toBe(true);
			if ("error" in got) expect(got.error).toContain("filter");
		});
	}
});

describe("describeResult", () => {
	const tests: { name: string; args: [number, boolean, number]; want: string }[] = [
		{ name: "a whole result", args: [12, false, 8], want: "12 rows in 8 ms" },
		{ name: "one row", args: [1, false, 3], want: "1 row in 3 ms" },
		{ name: "nothing", args: [0, false, 2], want: "0 rows in 2 ms" },
		// The truncation matters more than the count: 200 rows that are the first
		// 200 of 40,000 is a different answer, and must not read as the whole table.
		{
			name: "a truncated result says so",
			args: [200, true, 140],
			want: "first 200 rows — more were not read — in 140 ms",
		},
	];

	for (const test of tests) {
		it(test.name, () => {
			expect(describeResult(...test.args)).toBe(test.want);
		});
	}
});
