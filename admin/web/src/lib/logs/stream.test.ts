import { describe, expect, it } from "vitest";
import { appendBounded, isPinnedToBottom, logStreamUrl, splitLines } from "./stream";

describe("splitLines", () => {
	it("carries a partial line forward rather than rendering it", () => {
		// A chunk boundary lands mid-line often enough to matter. Rendering the
		// fragment would show every long line broken in two for one frame.
		const first = splitLines("", "alpha\nbeta\ngam");
		expect(first.lines).toEqual(["alpha", "beta"]);
		expect(first.pending).toBe("gam");

		const second = splitLines(first.pending, "ma\ndelta\n");
		expect(second.lines).toEqual(["gamma", "delta"]);
		expect(second.pending).toBe("");
	});

	it("yields nothing for a chunk with no newline", () => {
		const got = splitLines("", "still going");
		expect(got.lines).toEqual([]);
		expect(got.pending).toBe("still going");
	});

	it("keeps blank lines, which a stack trace is full of", () => {
		const got = splitLines("", "a\n\nb\n");
		expect(got.lines).toEqual(["a", "", "b"]);
	});
});

describe("appendBounded", () => {
	it("keeps the newest lines when the buffer overflows", () => {
		const existing = Array.from({ length: 5 }, (_, i) => `old-${i}`);
		const got = appendBounded(existing, ["new-0", "new-1"], 5);
		expect(got).toEqual(["old-2", "old-3", "old-4", "new-0", "new-1"]);
	});

	it("returns the same array when there is nothing to add", () => {
		const existing = ["a"];
		// Identity, not just equality: a new array every empty chunk would re-run
		// the autoscroll effect on every network read that carried no full line.
		expect(appendBounded(existing, [])).toBe(existing);
	});

	it("does not truncate below the limit", () => {
		expect(appendBounded(["a"], ["b"], 5)).toEqual(["a", "b"]);
	});
});

describe("isPinnedToBottom", () => {
	const tests: { name: string; args: [number, number, number]; want: boolean }[] = [
		{ name: "exactly at the bottom", args: [1000, 800, 200], want: true },
		// Not zero: a smooth scroll and a fractional device pixel ratio both leave
		// a pixel or two of slack, and an exact test would unpin constantly.
		{ name: "a few pixels of slack still counts", args: [1000, 790, 200], want: true },
		{ name: "scrolled up to read something", args: [1000, 400, 200], want: false },
		{ name: "content shorter than the viewport", args: [100, 0, 200], want: true },
	];

	for (const test of tests) {
		it(test.name, () => {
			expect(isPinnedToBottom(...test.args)).toBe(test.want);
		});
	}
});

describe("logStreamUrl", () => {
	it("omits what was not asked for", () => {
		expect(logStreamUrl({ namespace: "default", pod: "api-0" })).toBe(
			"/api/kubernetes/logs?namespace=default&pod=api-0",
		);
	});

	it("carries every option", () => {
		const got = new URL(
			logStreamUrl({
				namespace: "admin",
				pod: "api-7d9f-x4k2p",
				container: "api",
				follow: true,
				tail: 500,
				previous: true,
			}),
			"https://example.invalid",
		);
		expect(got.searchParams.get("container")).toBe("api");
		expect(got.searchParams.get("follow")).toBe("true");
		expect(got.searchParams.get("tail")).toBe("500");
		expect(got.searchParams.get("previous")).toBe("true");
	});

	it("encodes a name that would otherwise break the query", () => {
		const got = logStreamUrl({ namespace: "a&b", pod: "c=d" });
		expect(got).toContain("namespace=a%26b");
		expect(got).toContain("pod=c%3Dd");
	});
});
