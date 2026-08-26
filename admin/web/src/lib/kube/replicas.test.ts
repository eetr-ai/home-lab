import { describe, expect, it } from "vitest";
import { MAX_REPLICAS, parseReplicas } from "./replicas";

describe("parseReplicas", () => {
	const tests: { name: string; raw: string; want: number | null }[] = [
		{ name: "an ordinary count", raw: "3", want: 3 },
		// Scaling to zero is a real thing to want — but only when it was typed.
		{ name: "a typed zero", raw: "0", want: 0 },
		{ name: "surrounding whitespace is ignored", raw: " 5 ", want: 5 },
		{ name: "at the cap", raw: String(MAX_REPLICAS), want: MAX_REPLICAS },
		// The one that matters: Number("") is 0, so the obvious expression reads a
		// cleared field as a deliberate request to take the workload down.
		{ name: "a cleared field is not a zero", raw: "", want: null },
		{ name: "whitespace alone is not a zero", raw: "   ", want: null },
		{ name: "negative", raw: "-3", want: null },
		{ name: "fractional", raw: "1.5", want: null },
		{ name: "exponent notation", raw: "1e3", want: null },
		{ name: "not a number at all", raw: "three", want: null },
		{ name: "past the cap", raw: String(MAX_REPLICAS + 1), want: null },
	];

	for (const test of tests) {
		it(test.name, () => {
			expect(parseReplicas(test.raw)).toBe(test.want);
		});
	}
});
