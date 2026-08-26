"use client";

import { MessageSquareOff } from "lucide-react";

/**
 * A message the run took responsibility for and never answered.
 *
 * The other outcome no longer reaches here. A message that was *read* is written
 * into the transcript where it was read — see `takeIn` — because what followed was
 * said because of it, so it belongs in the conversation rather than as a line in
 * the middle of the reply. Nothing followed from one that was never reached, so
 * there is no position in the conversation to give it, and this is what is left:
 * the one case where something a person sent goes unanswered, said out loud.
 */
export default function SignalNotice({ signal, text }: { signal: string; text?: string }) {
	if (signal !== "unanswered") return null;
	return (
		<p className="flex items-start gap-1.5 text-xs text-warning-fg">
			<MessageSquareOff className="mt-px h-3.5 w-3.5 shrink-0" />
			{/* The text is what makes this actionable, and empty quotes would be worse
			    than none — so a frame without one says the same thing without them. */}
			<span>
				{text
					? `It ran out of steps before answering “${text}”. Ask again.`
					: "It ran out of steps before answering a message sent mid-answer. Ask again."}
			</span>
		</p>
	);
}
