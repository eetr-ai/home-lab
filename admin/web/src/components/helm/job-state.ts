/**
 * Folding a job's events into what the panel renders.
 *
 * Pure, and separate from the component for the reason the agent drawer's reducer
 * is: the ordering rules here are the feature, and they are only testable if no
 * React is in the way.
 */

import type { HelmJob, HelmJobPhase } from "@/lib/api/types";
import type { JobEvent } from "./job-events";

/**
 * How many log lines are kept.
 *
 * A chart with a chatty install hook can produce a great deal of output, and an
 * unbounded array in a tab left open is a memory leak with a deploy attached. The
 * whole log is still readable through the log endpoint.
 */
export const MAX_LINES = 2000;

export interface JobState {
	job: HelmJob | null;
	phase: HelmJobPhase;
	reason?: string;
	lines: string[];
	/** True once the operation has ended. Nothing reconnects after this. */
	terminal: boolean;
	/**
	 * Why the STREAM stopped, when it did.
	 *
	 * Deliberately not the same field as a failed operation. An API pod restarting
	 * is not a failed deploy, and a panel that showed one as the other would report
	 * a successful upgrade as broken — which, because upgrading the panel drops
	 * this stream every time, would be the common case rather than a rare one.
	 */
	streamError?: string;
	/** Lines were dropped from the front to stay under the cap. */
	truncated: boolean;
}

export function initialJobState(job: HelmJob | null = null): JobState {
	return {
		job,
		phase: job?.phase ?? "pending",
		reason: job?.reason,
		lines: [],
		terminal: job ? job.phase === "succeeded" || job.phase === "failed" : false,
		truncated: false,
	};
}

/**
 * Apply one event.
 *
 * The rules worth stating, because each is a case that would otherwise be wrong:
 *
 * - **A snapshot replaces, and clears the lines.** It arrives first on every
 *   connection, so a reconnect that appended would show the tail twice. Clearing
 *   and refilling from what the server resends is idempotent and needs no ids,
 *   which is what lets the whole design carry no per-connection state.
 * - **A log line after a terminal phase still appends.** The server drains the log
 *   before saying "done", but a late line is worth more than consistency here: it
 *   is usually the reason something failed.
 * - **`error` does not make the state terminal.** It means reconnect. Only `done`
 *   is the end.
 */
export function foldJobEvent(state: JobState, event: JobEvent): JobState {
	switch (event.type) {
		case "snapshot":
			return {
				job: event.job,
				phase: event.job.phase,
				reason: event.job.reason,
				lines: [],
				terminal: event.job.phase === "succeeded" || event.job.phase === "failed",
				truncated: false,
				streamError: undefined,
			};

		case "phase":
			return {
				...state,
				phase: event.phase,
				reason: event.reason ?? state.reason,
				job: state.job ? { ...state.job, phase: event.phase, pod: event.pod } : null,
				// A phase event never ends the stream on its own; `done` does. The
				// server sends both, and acting on the first would cut the log off.
				streamError: undefined,
			};

		case "log":
			return { ...state, ...appendLine(state, event.line) };

		case "done":
			return {
				...state,
				phase: event.phase,
				reason: event.reason ?? state.reason,
				terminal: true,
				streamError: undefined,
			};

		case "error":
			return { ...state, streamError: event.error };
	}
}

function appendLine(state: JobState, line: string): Pick<JobState, "lines" | "truncated"> {
	const lines = [...state.lines, line];
	if (lines.length <= MAX_LINES) return { lines, truncated: state.truncated };
	// Dropped from the front: the end of a log is the part that explains what
	// happened, and the beginning is a chart being pulled.
	return { lines: lines.slice(lines.length - MAX_LINES), truncated: true };
}
