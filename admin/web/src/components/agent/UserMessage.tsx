"use client";

import { Check, Loader2, MessageSquareOff } from "lucide-react";
import { answerOf, type Turn } from "./turns";

/**
 * What somebody said, and — when they said it mid-answer — whether the agent has
 * read it yet.
 *
 * A message sent while a run is in flight is not answered by the request that
 * carried it: the runtime hands it to the run already working and returns nothing,
 * so from this window it looks identical whether it was folded into the
 * conversation or thrown away. The run says which, on the stream that is already
 * open, and that is what these three states are.
 *
 * An ordinary question has no state to show. It started its own run, and the
 * answer appearing underneath it is the acknowledgement.
 */
export default function UserMessage({ turn }: { turn: Turn }) {
	return (
		<div className="flex flex-col items-end gap-0.5">
			<div
				// whitespace-pre-wrap because the composer takes Shift+Enter and a
				// pasted block, so without it a message someone wrote over four lines
				// is read back to them as one. break-words because a pod name or a URL
				// is a single unbreakable token wider than 85% of the drawer.
				className={`max-w-[85%] whitespace-pre-wrap break-words rounded-card px-3 py-1.5 text-sm text-brand-fg transition-colors ${
					// Held back until it is picked up, so a message waiting on a model call
					// does not sit there looking as settled as one already answered.
					turn.delivery === "pending" ? "bg-brand/50" : "bg-brand"
				}`}
			>
				{answerOf(turn)}
			</div>
			<DeliveryNote delivery={turn.delivery} />
		</div>
	);
}

function DeliveryNote({ delivery }: { delivery: Turn["delivery"] }) {
	switch (delivery) {
		case "pending":
			return (
				<Note>
					<Loader2 className="h-3 w-3 animate-spin" />
					Waiting for it to read this
				</Note>
			);

		// It is mid-turn and cannot reply to this separately, so the only honest
		// report is that it is now part of what it is working on.
		case "taken":
			return (
				<Note>
					<Check className="h-3 w-3" />
					Read, and taken into account
				</Note>
			);

		// The one case where something a person sent goes nowhere — the run was
		// stopped, or ran out of steps before reaching it. It must not be silent.
		case "missed":
			return (
				<Note tone="text-warning-fg">
					<MessageSquareOff className="h-3 w-3" />
					It never got to this one. Ask again.
				</Note>
			);

		default:
			return null;
	}
}

function Note({
	children,
	tone = "text-muted-foreground",
}: {
	children: React.ReactNode;
	tone?: string;
}) {
	return <span className={`flex items-center gap-1 pr-0.5 text-xs ${tone}`}>{children}</span>;
}
