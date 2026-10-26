import { describe, expect, it } from "vitest";
import type { BackupRun } from "@/lib/api/types";
import { isActive, parseScheduleEvent, runFinished } from "./schedule-events";

function run(name: string, phase: BackupRun["phase"]): BackupRun {
	return { name, phase, manual: true, operation: "backup", namespace: "admin", containers: ["upload"], createdAt: "2026-10-26T00:00:00Z" };
}

const schedule = { name: "weekly", cron: "0 3 * * 0", databases: [], folder: { id: "f", name: "F" }, keep: 7 };

describe("parseScheduleEvent", () => {
	it("reads a state", () => {
		const event = parseScheduleEvent("state", JSON.stringify({ schedule, runs: [run("a", "running")] }));
		expect(event?.type).toBe("state");
	});

	it("reads the stream failing, without trusting its shape", () => {
		expect(parseScheduleEvent("error", '{"error":"gone"}')).toEqual({ type: "error", error: "gone" });
		expect(parseScheduleEvent("error", "{}")).toEqual({ type: "error", error: "the stream failed" });
	});

	// A frame the page cannot draw from must not replace one it can.
	it.each([
		["a state with no runs", "state", JSON.stringify({ schedule })],
		["a state with runs as null", "state", JSON.stringify({ schedule, runs: null })],
		["a state with no schedule", "state", JSON.stringify({ runs: [] })],
		["not JSON", "state", "nope"],
		["an unknown event", "other", "{}"],
	])("ignores %s", (_, event, data) => {
		expect(parseScheduleEvent(event, data)).toBeNull();
	});
});

describe("runFinished", () => {
	it("sees a run go from running to done", () => {
		expect(runFinished([run("a", "running")], [run("a", "succeeded")])).toBe(true);
		expect(runFinished([run("a", "pending")], [run("a", "failed")])).toBe(true);
	});

	// A short run can start and finish between two readings.
	it("sees a run that arrived already finished", () => {
		expect(runFinished([], [run("a", "succeeded")])).toBe(true);
	});

	it("ignores runs that were already finished, and ones still going", () => {
		expect(runFinished([run("a", "succeeded")], [run("a", "succeeded")])).toBe(false);
		expect(runFinished([], [run("a", "running")])).toBe(false);
	});
});

describe("isActive", () => {
	it("is pending or running", () => {
		expect(isActive(run("a", "pending"))).toBe(true);
		expect(isActive(run("a", "running"))).toBe(true);
		expect(isActive(run("a", "failed"))).toBe(false);
	});
});
