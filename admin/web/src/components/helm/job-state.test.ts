import { describe, expect, it } from "vitest";
import { MAX_LINES, foldJobEvent, initialJobState } from "./job-state";
import type { JobEvent } from "./job-events";
import type { HelmJob } from "@/lib/api/types";

const job = (overrides: Partial<HelmJob> = {}): HelmJob =>
	({
		name: "helm-rollout-abcde",
		namespace: "apps",
		release: "podinfo",
		operation: "rollout",
		phase: "running",
		createdAt: "2026-08-29T00:00:00Z",
		...overrides,
	}) as HelmJob;

const fold = (events: JobEvent[]) => events.reduce(foldJobEvent, initialJobState());

describe("foldJobEvent", () => {
	it("follows an operation to its end", () => {
		const state = fold([
			{ type: "snapshot", job: job({ phase: "pending" }) },
			{ type: "phase", phase: "running", pod: "p-1" },
			{ type: "log", line: "Upgrading podinfo" },
			{ type: "done", phase: "succeeded" },
		]);

		expect(state.phase).toBe("succeeded");
		expect(state.terminal).toBe(true);
		expect(state.lines).toEqual(["Upgrading podinfo"]);
	});

	// A snapshot arrives first on EVERY connection, so a reconnect that appended
	// would show the tail twice. Clearing and refilling is what lets the design
	// carry no per-connection state on either side.
	it("clears the log on a snapshot rather than duplicating it", () => {
		const state = fold([
			{ type: "snapshot", job: job() },
			{ type: "log", line: "one" },
			{ type: "log", line: "two" },
			// the connection dropped and came back
			{ type: "snapshot", job: job() },
			{ type: "log", line: "one" },
			{ type: "log", line: "two" },
		]);

		expect(state.lines).toEqual(["one", "two"]);
	});

	// THE distinction. An API pod restarting is not a failed deploy, and because
	// upgrading the panel drops this stream every time, getting it wrong would
	// report a successful self-upgrade as broken.
	it("keeps a stream failure separate from a failed operation", () => {
		const dropped = fold([
			{ type: "snapshot", job: job() },
			{ type: "error", error: "the watch ended" },
		]);
		expect(dropped.streamError).toBe("the watch ended");
		expect(dropped.terminal).toBe(false);
		expect(dropped.phase).toBe("running");

		const failed = fold([
			{ type: "snapshot", job: job() },
			{ type: "done", phase: "failed", reason: "BackoffLimitExceeded" },
		]);
		expect(failed.terminal).toBe(true);
		expect(failed.phase).toBe("failed");
		expect(failed.streamError).toBeUndefined();
	});

	// Reconnecting after a dropped stream clears the error, or the panel would go
	// on saying it had lost the connection it just re-established.
	it("clears a stream error when the stream comes back", () => {
		const state = fold([
			{ type: "snapshot", job: job() },
			{ type: "error", error: "the watch ended" },
			{ type: "snapshot", job: job() },
		]);
		expect(state.streamError).toBeUndefined();
	});

	// The server drains the log before saying done, but a late line is worth more
	// than consistency: it is usually the reason something failed.
	it("still appends a log line that arrives after the end", () => {
		const state = fold([
			{ type: "snapshot", job: job() },
			{ type: "done", phase: "failed" },
			{ type: "log", line: "Error: timed out waiting for the condition" },
		]);

		expect(state.lines).toEqual(["Error: timed out waiting for the condition"]);
		expect(state.terminal).toBe(true);
	});

	// An unbounded array in a tab left open is a memory leak with a deploy
	// attached. Dropped from the front, because the end of a log is the part that
	// explains what happened.
	it("caps the log and drops from the front", () => {
		const events: JobEvent[] = [{ type: "snapshot", job: job() }];
		for (let i = 0; i < MAX_LINES + 10; i++) events.push({ type: "log", line: `line ${i}` });

		const state = fold(events);
		expect(state.lines).toHaveLength(MAX_LINES);
		expect(state.lines[0]).toBe("line 10");
		expect(state.lines.at(-1)).toBe(`line ${MAX_LINES + 9}`);
		expect(state.truncated).toBe(true);
	});

	it("starts terminal when handed a job that has already finished", () => {
		const state = initialJobState(job({ phase: "succeeded" }));
		expect(state.terminal).toBe(true);
		expect(state.phase).toBe("succeeded");
	});
});

// A phase event says nothing about the pod when it omits one. Overwriting with
// undefined loses the name the log pane and the status line are both showing.
it("keeps the known pod when a phase event omits it", () => {
	const state = fold([
		{ type: "snapshot", job: job({ pod: "helm-rollout-abcde-x7k2q" }) },
		{ type: "phase", phase: "succeeded" },
	]);

	expect(state.job?.pod).toBe("helm-rollout-abcde-x7k2q");
	expect(state.phase).toBe("succeeded");
});

// Following a different job must not inherit the last one's state — above all
// its terminal phase, which would render the new stream as already finished.
it("clears everything when the followed job changes", () => {
	const first = fold([
		{ type: "snapshot", job: job({ phase: "running" }) },
		{ type: "log", line: "the first job" },
		{ type: "done", phase: "succeeded" },
	]);
	expect(first.terminal).toBe(true);

	const second = foldJobEvent(first, { type: "reset" });
	expect(second.terminal).toBe(false);
	expect(second.lines).toEqual([]);
	expect(second.job).toBeNull();
});
