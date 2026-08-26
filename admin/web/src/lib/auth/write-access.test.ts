import { describe, expect, it } from "vitest";
import { permitsWrite, writeAllowlist } from "./write-access";

describe("writeAllowlist", () => {
	const tests: { name: string; raw: string | undefined; want: string[] }[] = [
		{ name: "unset", raw: undefined, want: [] },
		{ name: "blank", raw: "   ", want: [] },
		{ name: "one address", raw: "operator@example.invalid", want: ["operator@example.invalid"] },
		{
			name: "several, with whitespace and mixed case",
			raw: " One@Example.invalid , two@example.invalid ",
			want: ["one@example.invalid", "two@example.invalid"],
		},
		{ name: "stray separators", raw: ",,one@example.invalid,,", want: ["one@example.invalid"] },
	];

	for (const { name, raw, want } of tests) {
		it(name, () => {
			expect(writeAllowlist(raw)).toEqual(want);
		});
	}
});

describe("permitsWrite", () => {
	const allowed = ["one@example.invalid"];

	it("permits any signed-in operator when the allowlist is empty", () => {
		expect(permitsWrite([], "anyone@example.invalid")).toBe(true);
		expect(permitsWrite([], undefined)).toBe(true);
	});

	it("permits a listed address regardless of case", () => {
		expect(permitsWrite(allowed, "One@Example.Invalid")).toBe(true);
	});

	it("refuses an unlisted address", () => {
		expect(permitsWrite(allowed, "two@example.invalid")).toBe(false);
	});

	// A caller an allowlist cannot identify has not been allowed by it. Admitting
	// them would make a session with no email claim the way around the setting.
	it("refuses a caller with no email once an allowlist is set", () => {
		expect(permitsWrite(allowed, undefined)).toBe(false);
		expect(permitsWrite(allowed, "")).toBe(false);
	});
});
