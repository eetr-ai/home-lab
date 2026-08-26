import { describe, expect, it } from "vitest";
import { formatCores, levelFor, percentOf } from "./resources";

describe("formatCores", () => {
	const tests: { name: string; millis: number; want: string }[] = [
		{ name: "a fraction of a core stays in millicores", millis: 250, want: "250m" },
		{ name: "zero is a real reading, not a missing one", millis: 0, want: "0m" },
		{ name: "just under a core stays in millicores", millis: 999, want: "999m" },
		{ name: "exactly one core reads as one", millis: 1000, want: "1 core" },
		{ name: "a whole number of cores has no decimal", millis: 4000, want: "4 cores" },
		{ name: "a fractional total gets one decimal", millis: 5500, want: "5.5 cores" },
		{ name: "a negative reading is not a reading", millis: -1, want: "—" },
		{ name: "a non-number is not a reading", millis: Number.NaN, want: "—" },
	];

	for (const test of tests) {
		it(test.name, () => {
			expect(formatCores(test.millis)).toBe(test.want);
		});
	}
});

describe("percentOf", () => {
	const tests: { name: string; part: number; total: number; want: number | null }[] = [
		{ name: "a half", part: 50, total: 100, want: 50 },
		{ name: "nothing used", part: 0, total: 100, want: 0 },
		// A meter with no denominator has nothing to fill. Zero would claim a
		// measurement that was never taken.
		{ name: "no denominator has no answer", part: 10, total: 0, want: null },
		{ name: "a negative denominator has no answer", part: 10, total: -5, want: null },
		{ name: "a negative part has no answer", part: -1, total: 100, want: null },
		{ name: "overcommitted is capped at full", part: 150, total: 100, want: 100 },
	];

	for (const test of tests) {
		it(test.name, () => {
			expect(percentOf(test.part, test.total)).toBe(test.want);
		});
	}
});

describe("levelFor", () => {
	const tests: { name: string; percent: number | null; want: string }[] = [
		{ name: "quiet", percent: 10, want: "normal" },
		{ name: "just below the warning", percent: 74.9, want: "normal" },
		{ name: "at the warning", percent: 75, want: "warning" },
		{ name: "just below critical", percent: 89.9, want: "warning" },
		{ name: "at critical", percent: 90, want: "critical" },
		{ name: "full", percent: 100, want: "critical" },
		// No reading is not a calm reading, but it has no severity of its own —
		// the tile says "unavailable" instead of colouring a bar it cannot draw.
		{ name: "no reading is not alarming", percent: null, want: "normal" },
	];

	for (const test of tests) {
		it(test.name, () => {
			expect(levelFor(test.percent)).toBe(test.want);
		});
	}
});
