"use client";

import { Loader2 } from "lucide-react";
import type { Turn } from "./turns";

/**
 * What the agent is doing, said where you can always see it.
 *
 * The only other sign of life is inside the scroller, which means it scrolls out
 * of view the moment there is anything to scroll — exactly when a run is long
 * enough to wonder about. This lives outside the scroller, above the composer, and
 * stays put.
 *
 * It says what the agent is doing rather than that it is doing something. "Running
 * admin_read" and "Shortening the conversation" are different waits with different
 * expected lengths, and a reader who knows which one they are in does not have to
 * guess whether anything is wrong.
 */
export default function WorkingStatus({ turn }: { turn: Turn | undefined }) {
	const label = statusOf(turn);
	if (!label) return null;

	return (
		<div className="flex items-center gap-2 px-3 pt-2 text-xs text-muted-foreground">
			<Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin" />
			<span className="truncate">{label}</span>
		</div>
	);
}

/** What the run is doing right now, from the last thing it reported. */
function statusOf(turn: Turn | undefined): string | null {
	if (!turn?.streaming) return null;

	const last = turn.segments.at(-1);
	if (!last) return "Thinking…";

	switch (last.kind) {
		case "thinking":
			return "Thinking…";
		case "tools": {
			const open = last.runs.find((run) => !run.done);
			// Every call answered and the turn still open means the agent is back at
			// the model with what they returned — which is the slow part, and worth
			// saying rather than leaving as an unchanged "Running".
			return open ? `Running ${open.tool}…` : "Reading the results…";
		}
		case "compaction":
			return last.done ? "Thinking…" : "Shortening the conversation…";
		case "signal":
			return "Taking your message into account…";
		case "text":
			return "Answering…";
	}
}
