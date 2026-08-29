/**
 * Reading a Helm job's event stream: a parser per frame that trusts none of it.
 *
 * The SSE framing underneath is in `lib/sse.ts`, shared with the agent drawer.
 * This is only the `data` of each frame, and it follows the same rule that
 * module's sibling does — return null for anything unusable rather than throwing,
 * because a stream is a bad place to discover a type error.
 */

import type { HelmJob, HelmJobPhase } from "@/lib/api/types";

/** The event names the API sends. */
export const JOB_EVENTS = {
	/** The whole job. First on every connection, including a reconnect. */
	snapshot: "snapshot",
	phase: "phase",
	log: "log",
	/** The operation ended. The server closes after this. */
	done: "done",
	/** The STREAM failed. Not the operation — see below. */
	error: "error",
} as const;

const PHASES: readonly string[] = ["pending", "running", "succeeded", "failed"];

const isPhase = (value: unknown): value is HelmJobPhase =>
	typeof value === "string" && PHASES.includes(value);

const str = (value: unknown): string | undefined =>
	typeof value === "string" && value !== "" ? value : undefined;

/** One parsed event from a job's stream. */
export type JobEvent =
	| { type: "snapshot"; job: HelmJob }
	| { type: "phase"; phase: HelmJobPhase; reason?: string; pod?: string }
	| { type: "log"; line: string }
	| { type: "done"; phase: HelmJobPhase; reason?: string }
	| { type: "error"; error: string };

/**
 * Parse one frame of a job stream.
 *
 * Every field the panel uses is checked, not just the shape. A `phase` that is
 * not one of the four would flow into a badge and a comparison and read as
 * "still running" forever; a non-string `line` reaches React as an object child
 * and takes the panel down.
 */
export function parseJobEvent(event: string, data: string): JobEvent | null {
	let parsed: unknown;
	try {
		parsed = JSON.parse(data);
	} catch {
		return null;
	}
	if (!parsed || typeof parsed !== "object") return null;
	const frame = parsed as Record<string, unknown>;

	switch (event) {
		case JOB_EVENTS.snapshot: {
			const job = frame.job;
			if (!job || typeof job !== "object") return null;
			if (!isPhase((job as Record<string, unknown>).phase)) return null;
			return { type: "snapshot", job: job as unknown as HelmJob };
		}

		case JOB_EVENTS.phase:
			if (!isPhase(frame.phase)) return null;
			return {
				type: "phase",
				phase: frame.phase,
				reason: str(frame.reason),
				pod: str(frame.pod),
			};

		case JOB_EVENTS.log:
			// An empty line is a real line — a blank between two stanzas of Helm
			// output — so this checks the type rather than the truthiness.
			if (typeof frame.line !== "string") return null;
			return { type: "log", line: frame.line };

		case JOB_EVENTS.done:
			if (!isPhase(frame.phase)) return null;
			return { type: "done", phase: frame.phase, reason: str(frame.reason) };

		case JOB_EVENTS.error: {
			const error = str(frame.error);
			if (!error) return null;
			return { type: "error", error };
		}

		default:
			// An event this build has never heard of is a newer API, not a failure.
			// Skipping it beats throwing mid-stream.
			return null;
	}
}
