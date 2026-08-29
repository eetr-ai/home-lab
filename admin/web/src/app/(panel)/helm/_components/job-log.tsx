"use client";

import { useEffect, useRef } from "react";

/**
 * Helm's own output, as it arrives.
 *
 * Sticks to the bottom only while the operator is already there. Scrolling up
 * during a deploy means reading something, and yanking the view back down on the
 * next line is the behaviour that makes a log pane unusable.
 */
export function JobLog({ lines, truncated }: { lines: string[]; truncated: boolean }) {
	const pane = useRef<HTMLPreElement>(null);
	const stuck = useRef(true);

	useEffect(() => {
		const element = pane.current;
		if (!element || !stuck.current) return;
		element.scrollTop = element.scrollHeight;
	}, [lines]);

	function onScroll() {
		const element = pane.current;
		if (!element) return;
		// A few pixels of tolerance: sub-pixel scroll heights mean an exact
		// comparison is false on a pane that is visually at the bottom.
		const distance = element.scrollHeight - element.scrollTop - element.clientHeight;
		stuck.current = distance < 24;
	}

	if (lines.length === 0) return null;

	return (
		<div className="flex flex-col gap-1">
			{truncated && (
				<p className="text-xs text-muted-foreground">
					Showing the most recent output. The whole log is on the job itself.
				</p>
			)}
			<pre
				ref={pane}
				onScroll={onScroll}
				className="max-h-80 overflow-auto rounded-card bg-surface-sunken p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap"
			>
				{lines.join("\n")}
			</pre>
		</div>
	);
}
