"use client";

import { useState } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { ArrowDown, MessageSquarePlus, X } from "lucide-react";
import { Banner, IconButton } from "@/components/ui";
import AgentMessage from "./AgentMessage";
import ContextGauge from "./ContextGauge";
import Composer from "./Composer";
import WorkingStatus from "./WorkingStatus";
import { useAgentChat } from "./useAgentChat";
import { useStickToBottom } from "./useStickToBottom";

/**
 * The conversation: transcript, composer, and the navigation the agent asks for.
 *
 * It renders inside the shell's flex row rather than over the page — see
 * agent-shell.tsx for the column it sits in and why. This component owns the
 * conversation and nothing about where it is; it has no width, no position and no
 * animation of its own.
 *
 * Deliberately **not** a SidePanel. That primitive portals to the body, draws a
 * scrim, traps focus, locks body scroll and declares `role="dialog"
 * aria-modal="true"` — every one of which is wrong here. The page beside this
 * stays live and clickable, because the agent navigates: asking it to take you to
 * a workload and watching the page change beside the answer is the point.
 */
export default function AgentDrawer({
	userKey,
	onCollapse,
}: {
	userKey: string;
	onCollapse: () => void;
}) {
	const router = useRouter();
	const pathname = usePathname();
	// The query string is part of where somebody is, not decoration on it: these
	// pages carry their scope there, so /postgres/query and
	// /postgres/query?database=orders are two different questions to be asked "what
	// am I looking at". Sent whole, so the agent can answer about the database in
	// front of the operator rather than the first one in the list.
	const search = useSearchParams().toString();
	const page = search ? `${pathname}?${search}` : pathname;
	const [draft, setDraft] = useState("");

	const chat = useAgentChat(userKey, page, (target) => {
		// The path was already checked for being site-relative when the frame was
		// parsed — see parseNavigateEvent, which is the boundary. router.push keeps
		// it a client-side navigation, so this drawer and the conversation in it
		// survive the move.
		router.push(target.path);
	});

	const { ref: scroller, following, toBottom } = useStickToBottom(chat.turns);
	// The last *agent* turn, not the last turn. A message sent mid-answer is
	// appended while the run continues, so the end of the array is a question the
	// reader just typed — with no gauge on it and nothing for the status strip to
	// report, which would blank both at the moment there is most to say.
	const open = chat.turns.findLast((turn) => turn.role === "agent");

	const submit = () => {
		chat.send(draft);
		setDraft("");
	};

	return (
		<div className="flex h-full min-h-0 flex-col bg-surface text-foreground">
			<header className="flex shrink-0 items-center gap-2 border-b border-border px-3 py-2">
				<span className="text-sm font-semibold">Sous</span>
				<span className="truncate text-xs text-muted-foreground">at your service</span>
				<div className="ml-auto flex items-center gap-1">
					{open?.context && <ContextGauge gauge={open.context} />}
					<IconButton
						type="button"
						onClick={chat.reset}
						title="New conversation"
						aria-label="New conversation"
					>
						<MessageSquarePlus className="h-4 w-4" />
					</IconButton>
					<IconButton type="button" onClick={onCollapse} title="Close" aria-label="Close">
						<X className="h-4 w-4" />
					</IconButton>
				</div>
			</header>

			<div className="relative flex min-h-0 flex-1 flex-col">
				<div ref={scroller} className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto px-3 py-3">
					{chat.turns.length === 0 && <Empty />}

					{chat.turns.map((turn) => (
						<AgentMessage key={turn.id} turn={turn} />
					))}

					{/* A banner inside the section it relates to, as every other error in
					    this panel is. There is no page-level alternative here — the page
					    beside the drawer is somebody else's. */}
					<Banner variant="error" message={chat.error} />
				</div>

				{/* Offered only when following is off, so it is a way back rather than a
				    permanent control — and it is the only sign that scrolling away turned
				    anything off. */}
				{!following && (
					<button
						type="button"
						onClick={toBottom}
						aria-label="Jump to the latest"
						className="absolute right-4 bottom-2 flex items-center gap-1 rounded-full border border-border bg-surface px-2 py-1 text-xs text-muted-foreground shadow-md"
					>
						<ArrowDown className="h-3 w-3" />
						Latest
					</button>
				)}
			</div>

			<div className="shrink-0">
				<WorkingStatus turn={open} />
				<Composer
					draft={draft}
					onDraft={setDraft}
					onSubmit={submit}
					busy={chat.busy}
					onStop={chat.stop}
				/>
			</div>
		</div>
	);
}

function Empty() {
	return (
		<div className="m-auto max-w-80 text-center text-xs text-muted-foreground">
			<p>
				Ask about this cluster — a pod that will not start, what is on a node, which databases
				exist.
			</p>
			<p className="mt-2">
				It knows which page you are on, and can take you to another one. It reads; the changes
				are yours to make.
			</p>
		</div>
	);
}
