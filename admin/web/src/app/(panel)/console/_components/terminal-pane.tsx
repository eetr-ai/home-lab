"use client";

import { useEffect, useRef } from "react";
import { RotateCw } from "lucide-react";
import { Button, Spinner, cn } from "@/components/ui";
import { useTtydSocket } from "./use-ttyd-socket";
import { useXterm } from "./use-xterm";

interface TerminalPaneProps {
	name: string;
	socketBase: string;
	/** Hidden panes stay mounted and connected; only the active one is shown. */
	visible: boolean;
	/** The shell went away on purpose; the tab should close. */
	onEnded: () => void;
}

const MESSAGES = {
	dropped: "The connection dropped. The session is still running in the console.",
	refused:
		"The console did not answer. It may still be starting, or your sign-in lapsed — reload the page if reconnecting does not help.",
} as const;

/** One session's terminal, with its connection state laid over it when it is not live. */
export function TerminalPane({ name, socketBase, visible, onEnded }: TerminalPaneProps) {
	const host = useRef<HTMLDivElement>(null);
	const xterm = useXterm(host);
	const { status, reconnect } = useTtydSocket(xterm, name, socketBase, onEnded);

	// A pane that was hidden was never fitted to its real size; do it on showing.
	useEffect(() => {
		if (!visible || !xterm) return;
		xterm.fit.fit();
		xterm.term.focus();
	}, [visible, xterm]);

	return (
		<div className={cn("absolute inset-0 p-2", visible ? "block" : "hidden")} aria-hidden={!visible}>
			<div ref={host} className="h-full w-full" />
			{status === "connecting" ? (
				<div className="absolute right-3 top-3">
					<Spinner />
				</div>
			) : null}
			{status === "dropped" || status === "refused" ? (
				<div className="absolute inset-0 flex items-center justify-center bg-scrim p-6">
					<div className="flex max-w-md flex-col items-center gap-3 rounded-card bg-surface p-5 text-center text-sm text-foreground">
						<p>{MESSAGES[status]}</p>
						<Button type="button" variant="secondary" icon={RotateCw} onClick={reconnect}>
							Reconnect
						</Button>
					</div>
				</div>
			) : null}
		</div>
	);
}
