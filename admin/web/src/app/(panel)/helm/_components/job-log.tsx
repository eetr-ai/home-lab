"use client";

import { useEffect, useId, useRef, useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";

/**
 * Helm's own output, as it arrives.
 *
 * Sticks to the bottom only while the operator is already there. Scrolling up
 * during a deploy means reading something, and yanking the view back down on the
 * next line is the behaviour that makes a log pane unusable.
 *
 * Collapsible, and open to begin with: while an operation runs the output is the
 * reason the panel is on screen, so hiding it by default would make every deploy
 * start with a click. Once it has finished — and the pane keeps its last job
 * until another one starts — it is a tall block of text between the header and
 * everything below, and folding it away is the cheaper of the two.
 */
export function JobLog({ lines, truncated }: { lines: string[]; truncated: boolean }) {
	const pane = useRef<HTMLPreElement>(null);
	const stuck = useRef(true);
	const [open, setOpen] = useState(true);
	const paneId = useId();

	useEffect(() => {
		const element = pane.current;
		if (!element || !stuck.current) return;
		element.scrollTop = element.scrollHeight;
	}, [lines, open]);

	function onScroll() {
		const element = pane.current;
		if (!element) return;
		// A few pixels of tolerance: sub-pixel scroll heights mean an exact
		// comparison is false on a pane that is visually at the bottom.
		const distance = element.scrollHeight - element.scrollTop - element.clientHeight;
		stuck.current = distance < 24;
	}

	if (lines.length === 0) return null;

	const Chevron = open ? ChevronDown : ChevronRight;

	return (
		<div className="flex flex-col gap-1">
			{/* The line count is what makes the collapsed state worth reading: it
			    says there is output without making you unfold it to find out. */}
			<button
				type="button"
				onClick={() => setOpen((wasOpen) => !wasOpen)}
				aria-expanded={open}
				aria-controls={paneId}
				className="flex items-center gap-1.5 self-start rounded-control text-xs text-muted-foreground hover:text-foreground"
			>
				<Chevron className="h-3.5 w-3.5 shrink-0" />
				Helm output
				<span className="tabular-nums">({lines.length} lines)</span>
			</button>

			{open && truncated && (
				<p className="text-xs text-muted-foreground">
					Showing the most recent output. The whole log is on the job itself.
				</p>
			)}
			{/* Focusable and labelled: a pane that scrolls has to be reachable
			    without a mouse, and `log` tells a screen reader that content
			    arrives here over time rather than all at once. Unmounted rather
			    than hidden when collapsed — a `role="log"` a screen reader can
			    still reach is a pane that keeps announcing lines nobody asked to
			    see. */}
			{open && (
				<pre
					id={paneId}
					ref={pane}
					onScroll={onScroll}
					tabIndex={0}
					role="log"
					aria-label="Helm output"
					className="max-h-80 overflow-auto rounded-card bg-surface-sunken p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap"
				>
					{lines.join("\n")}
				</pre>
			)}
		</div>
	);
}
