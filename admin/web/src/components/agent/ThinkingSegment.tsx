"use client";

import { useEffect, useRef, useState } from "react";
import { Brain, ChevronDown, ChevronRight } from "lucide-react";

/**
 * How close to the end of the reasoning box still counts as being at it.
 *
 * Deliberately smaller than the transcript's own slack rather than shared with
 * it. This box is `max-h-40` — 160px — so the transcript's 48 would be nearly a
 * third of it, and a reader who scrolled up a couple of lines would still count
 * as being at the bottom and be pulled back down. About one line is right here.
 */
const BOTTOM_SLACK = 24;

/**
 * The model's reasoning while it is reasoning.
 *
 * This exists because leaving it out makes the panel look broken: the agent runs
 * with reasoning on, so most of what a run produces is thinking, and without it
 * there is nothing on screen between the question and the answer — a model that is
 * working is indistinguishable from one that has hung.
 *
 * Open while it streams, closed once something follows it. That ordering is the
 * whole design: the reasoning is worth watching when it is the only thing
 * happening, and is clutter the moment there is a tool call or an answer to read
 * instead.
 *
 * One of these per stretch of reasoning rather than one per turn: an agent that
 * thinks, calls a tool and thinks again did two separate pieces of thinking, and
 * the second is about what the first turned up.
 */
export default function ThinkingSegment({
	text,
	streaming,
	answered,
}: {
	text: string;
	/** The run is still going. */
	streaming: boolean;
	/** Something came after this, so the reasoning is no longer the main event. */
	answered: boolean;
}) {
	// Derived rather than synchronised: until someone touches it the panel simply
	// *is* open-while-unanswered, which needs no effect and cannot lag a render
	// behind. After a deliberate choice it follows that choice, so the answer
	// arriving never collapses a panel somebody is reading.
	const [choice, setChoice] = useState<boolean | null>(null);
	const open = choice ?? !answered;
	const body = useRef<HTMLDivElement>(null);

	// Follow the reasoning as it arrives, so the newest line is the visible one —
	// by scrolling this box and nothing else. `scrollIntoView` would scroll the
	// nearest scrollable ancestor, which is the transcript: every token would drag
	// the whole conversation down, quietly undoing the one thing the transcript's
	// own scrolling is careful about.
	//
	// And follow only while the reader is already at the end. This runs on every
	// token, so unconditionally it fights anyone who scrolls up inside the box to
	// re-read a line: the next token drags them back down. The transcript makes
	// the same distinction in useStickToBottom, for the same reason — scrolling
	// away is a deliberate act and streaming is not a reason to undo it.
	useEffect(() => {
		const el = body.current;
		if (!open || !el) return;
		const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < BOTTOM_SLACK;
		if (atBottom) el.scrollTop = el.scrollHeight;
	}, [text, open]);

	if (!text) return null;
	const Chevron = open ? ChevronDown : ChevronRight;

	return (
		<div className="rounded-card border border-border bg-surface-sunken">
			<button
				type="button"
				onClick={() => setChoice(!open)}
				aria-expanded={open}
				className="flex w-full items-center gap-1.5 px-2 py-1.5 text-left text-xs font-medium text-muted-foreground hover:text-foreground"
			>
				<Chevron className="h-3.5 w-3.5 shrink-0" />
				<Brain
					className={`h-3.5 w-3.5 shrink-0 ${streaming && !answered ? "animate-pulse text-brand" : ""}`}
				/>
				<span>{streaming && !answered ? "Thinking…" : "Thought about it"}</span>
				{!open && (
					<span className="ml-auto font-mono text-xs tabular-nums opacity-60">
						{/* A rough size, so a collapsed panel still says how much is behind it. */}
						{text.length.toLocaleString()} chars
					</span>
				)}
			</button>

			{open && (
				<div ref={body} className="max-h-40 overflow-y-auto px-2 pb-2">
					<p className="text-xs leading-relaxed whitespace-pre-wrap text-muted-foreground">
						{text}
					</p>
				</div>
			)}
		</div>
	);
}
