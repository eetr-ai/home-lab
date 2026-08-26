"use client";

import { Send, Square } from "lucide-react";
import { IconButton } from "@/components/ui";
import { useAutoGrow } from "./useAutoGrow";

/**
 * How many lines the box grows to before it starts scrolling instead.
 *
 * Four rather than one because a question worth asking this panel is usually
 * longer than a line, and four rather than more because the drawer's transcript
 * pays for every line — the composer grows downward out of the reading area.
 */
const MAX_ROWS = 4;

/**
 * The message box.
 *
 * Its own component because this is the part with the keyboard conventions in it,
 * and because the drawer around it is already at its line budget.
 */
export default function Composer({
	draft,
	onDraft,
	onSubmit,
	busy,
	onStop,
}: {
	draft: string;
	onDraft: (value: string) => void;
	onSubmit: () => void;
	/** A run is in flight. */
	busy: boolean;
	onStop: () => void;
}) {
	// The one rule both ways in are held to. The send button is disabled on an
	// empty draft; without this, Enter would not be — and whitespace would send.
	const submit = () => {
		if (draft.trim()) onSubmit();
	};

	// The height is owned by the hook, so there is no max-height class below: a
	// class and a measured cap would be two answers to one question, and the one
	// written inline is the one that would quietly stop matching.
	const box = useAutoGrow(draft, MAX_ROWS);

	return (
		<form
			onSubmit={(e) => {
				e.preventDefault();
				submit();
			}}
			className="flex items-end gap-2 border-t border-border px-3 py-2"
		>
			<textarea
				ref={box}
				value={draft}
				rows={1}
				placeholder="Ask about the cluster…"
				aria-label="Message"
				onChange={(e) => onDraft(e.target.value)}
				onKeyDown={(e) => {
					// Enter sends, shift+enter breaks the line — the convention every chat
					// input follows, and the one a multi-line paste needs.
					//
					// Except mid-composition. An IME uses Enter to accept the candidate it
					// is offering, so without this check anyone typing Japanese, Chinese or
					// Korean sends a half-finished word every time they choose one.
					if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
						e.preventDefault();
						submit();
					}
				}}
				className="min-h-8 flex-1 resize-none rounded-control border border-border bg-transparent px-2 py-1.5 text-sm outline-none focus:border-border-strong"
			/>
			{/* Kept mounted alongside Send rather than replacing it, so the control a
			    reader is aiming at does not move under the cursor when a run starts. */}
			{busy && (
				<IconButton type="button" aria-label="Stop" title="Stop" onClick={onStop}>
					<Square className="h-4 w-4" />
				</IconButton>
			)}
			<IconButton type="submit" aria-label="Send" title="Send" disabled={!draft.trim()}>
				<Send className="h-4 w-4" />
			</IconButton>
		</form>
	);
}
