import { describe, expect, it } from "vitest";
import { parseJobEvent } from "./job-events";

const snapshot = (phase: string) =>
	JSON.stringify({ job: { name: "helm-rollout-abcde", phase } });

describe("parseJobEvent", () => {
	it("reads a snapshot", () => {
		const event = parseJobEvent("snapshot", snapshot("running"));
		expect(event).toEqual({
			type: "snapshot",
			job: { name: "helm-rollout-abcde", phase: "running" },
		});
	});

	it("reads a phase change with its pod", () => {
		expect(parseJobEvent("phase", '{"phase":"running","pod":"p-1"}')).toEqual({
			type: "phase",
			phase: "running",
			reason: undefined,
			pod: "p-1",
		});
	});

	it("reads a terminal event with its reason", () => {
		expect(parseJobEvent("done", '{"phase":"failed","reason":"DeadlineExceeded"}')).toEqual({
			type: "done",
			phase: "failed",
			reason: "DeadlineExceeded",
		});
	});

	// A phase the panel does not know would flow into a badge and into the
	// comparison that decides whether to keep listening, and read as "still
	// running" forever.
	it("refuses a phase that is not one of the four", () => {
		expect(parseJobEvent("phase", '{"phase":"halfway"}')).toBeNull();
		expect(parseJobEvent("phase", '{"phase":42}')).toBeNull();
		expect(parseJobEvent("snapshot", snapshot("halfway"))).toBeNull();
	});

	// A blank line between two stanzas of Helm output is a real line, so this
	// checks the type rather than the truthiness.
	it("keeps an empty log line", () => {
		expect(parseJobEvent("log", '{"line":""}')).toEqual({ type: "log", line: "" });
	});

	// A non-string reaches React as an object child and takes the panel down.
	it("refuses a log line that is not a string", () => {
		expect(parseJobEvent("log", '{"line":{"nested":true}}')).toBeNull();
		expect(parseJobEvent("log", "{}")).toBeNull();
	});

	it("refuses an error event with nothing to say", () => {
		expect(parseJobEvent("error", '{"error":""}')).toBeNull();
		expect(parseJobEvent("error", '{"error":"the watch ended"}')).toEqual({
			type: "error",
			error: "the watch ended",
		});
	});

	// Skipping beats throwing mid-stream: an event this build has not heard of is
	// a newer API, not a failure.
	it("skips an unknown event and malformed JSON", () => {
		expect(parseJobEvent("something-new", "{}")).toBeNull();
		expect(parseJobEvent("phase", "not json")).toBeNull();
		expect(parseJobEvent("phase", "null")).toBeNull();
	});
});
