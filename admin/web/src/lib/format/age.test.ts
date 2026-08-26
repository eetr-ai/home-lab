import { describe, expect, it } from "vitest";
import { formatAge } from "./age";

const now = new Date("2026-08-25T12:00:00Z");

/** A timestamp `seconds` before `now`. */
function ago(seconds: number): string {
	return new Date(now.getTime() - seconds * 1000).toISOString();
}

describe("formatAge", () => {
	const tests: { name: string; timestamp: string; want: string }[] = [
		{ name: "seconds", timestamp: ago(12), want: "12s" },
		{ name: "just under a minute", timestamp: ago(59), want: "59s" },
		{ name: "a minute exactly", timestamp: ago(60), want: "1m" },
		{ name: "minutes round down", timestamp: ago(119), want: "1m" },
		{ name: "an hour exactly", timestamp: ago(3600), want: "1h" },
		{ name: "just under a day", timestamp: ago(86_399), want: "23h" },
		{ name: "a day exactly", timestamp: ago(86_400), want: "1d" },
		{ name: "days", timestamp: ago(9 * 86_400), want: "9d" },
		{ name: "a year", timestamp: ago(400 * 86_400), want: "1y" },
		// Two clocks disagreeing, not an age.
		{ name: "the future", timestamp: ago(-30), want: "0s" },
		{ name: "unparseable", timestamp: "not a date", want: "—" },
		{ name: "empty", timestamp: "", want: "—" },
	];

	for (const { name, timestamp, want } of tests) {
		it(name, () => {
			expect(formatAge(timestamp, now)).toBe(want);
		});
	}
});
