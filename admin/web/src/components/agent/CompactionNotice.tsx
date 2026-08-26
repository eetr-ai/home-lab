"use client";

import { Scissors } from "lucide-react";

/**
 * The agent shrinking its own conversation to stay inside its budget.
 *
 * Worth a line on screen because it is the one thing an agent does to *itself*
 * rather than to the model or a tool, and because it is slow: the summarize
 * strategy is a real model call, so without this the panel shows several seconds
 * of nothing and no reason for it.
 *
 * It is also the explanation for a thing people notice later — a long conversation
 * where the beginning has been forgotten. Saying it at the moment it happens is
 * much kinder than leaving it to be discovered.
 */
export default function CompactionNotice({
	done,
	dropped,
}: {
	done: boolean;
	/** How many messages went. Absent while it is still running. */
	dropped?: number;
}) {
	return (
		<p className="flex items-center gap-1.5 text-xs text-muted-foreground">
			<Scissors className={`h-3.5 w-3.5 shrink-0 ${done ? "" : "animate-pulse text-brand"}`} />
			{done ? (
				<span>
					Shortened the conversation to stay in budget
					{dropped ? ` — ${dropped.toLocaleString()} earlier messages summarised` : ""}.
				</span>
			) : (
				<span>Shortening the conversation to stay in budget…</span>
			)}
		</p>
	);
}
