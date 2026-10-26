import type { BackupRun, BackupSchedule } from "@/lib/api/types";

/**
 * The schedule stream's events, and what the page does with them.
 *
 * Pure, so it is tested without a stream: the hook that reads one is beside the
 * page, and all it decides is when to reconnect.
 */

export interface ScheduleState {
	schedule: BackupSchedule;
	runs: BackupRun[];
}

export type ScheduleEvent =
	| { type: "state"; state: ScheduleState }
	| { type: "error"; error: string };

/**
 * Read one SSE frame. Anything that is not a state with a schedule and a list of
 * runs is ignored rather than trusted — a frame the page cannot draw from must
 * not replace one it can.
 */
export function parseScheduleEvent(event: string, data: string): ScheduleEvent | null {
	let payload: unknown;
	try {
		payload = JSON.parse(data);
	} catch {
		return null;
	}

	if (event === "error") {
		const error = (payload as { error?: unknown })?.error;
		return { type: "error", error: typeof error === "string" ? error : "the stream failed" };
	}
	if (event !== "state") return null;

	const state = payload as Partial<ScheduleState>;
	if (!state?.schedule || typeof state.schedule.name !== "string" || !Array.isArray(state.runs)) {
		return null;
	}
	return { type: "state", state: state as ScheduleState };
}

/** Whether a run is still doing something. */
export function isActive(run: BackupRun): boolean {
	return run.phase === "pending" || run.phase === "running";
}

/**
 * Whether a run finished between two states — the moment the page refreshes
 * what is kept in Drive, since a finished run is what changes it.
 *
 * A run that was active and is not, or one that arrived already finished, which
 * is what a short run between two readings looks like.
 */
export function runFinished(previous: BackupRun[], next: BackupRun[]): boolean {
	const before = new Map(previous.map((run) => [run.name, run]));
	return next.some((run) => {
		if (isActive(run)) return false;
		const was = before.get(run.name);
		return was === undefined || isActive(was);
	});
}
