import { describe, expect, it } from "vitest";
import { formatUptime, hitRate, memoryTone } from "./overview";

const MB = 1024 * 1024;

describe("memoryTone", () => {
	it.each([
		["well under the ceiling", 10 * MB, 128 * MB, 0, "normal"],
		["at three quarters", 96 * MB, 128 * MB, 0, "warning"],
		["at nine tenths", 116 * MB, 128 * MB, 0, "danger"],
		// No ceiling means nothing to approach, however much is in use.
		["with no ceiling", 900 * MB, 0, 0, "normal"],
		// The count is since start, so a server that refused writes an hour ago
		// and has since freed memory still lost them. That stays visible.
		["after a refused write, even when empty now", 1 * MB, 128 * MB, 3, "danger"],
	] as const)("is right %s", (_, used, max, oom, tone) => {
		expect(memoryTone(used, max, oom)).toBe(tone);
	});
});

describe("hitRate", () => {
	it("is null before any lookup, rather than a zero nobody measured", () => {
		expect(hitRate(0, 0)).toBeNull();
	});

	it("is the share of lookups that hit", () => {
		expect(hitRate(90, 10)).toBe(90);
	});
});

describe("formatUptime", () => {
	it.each([
		[59, "0m"],
		[3_660, "1h 1m"],
		[90_000, "1d 1h"],
		[-1, "—"],
	])("%i seconds reads as %s", (seconds, text) => {
		expect(formatUptime(seconds)).toBe(text);
	});
});
