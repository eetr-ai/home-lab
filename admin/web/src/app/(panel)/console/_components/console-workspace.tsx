"use client";

import { useEffect, useReducer } from "react";
import { Plus, SquareTerminal } from "lucide-react";
import { Button, EmptyState } from "@/components/ui";
import { restoreSessions, sessionsReducer, STORAGE_KEY } from "@/lib/console/sessions";
import { SessionTabs } from "./session-tabs";
import { TerminalPane } from "./terminal-pane";

/** Remembered tabs, or none when storage is unavailable (private mode). */
function readStored(): string | null {
	try {
		return localStorage.getItem(STORAGE_KEY);
	} catch {
		return null;
	}
}

/**
 * The tab strip and the terminals under it.
 *
 * Every open tab keeps its terminal mounted, hidden when it is not the active one,
 * so switching tabs is instant and does not reconnect. Closing a tab detaches from
 * its tmux session; the session keeps running in the pod until something in it
 * exits.
 */
export function ConsoleWorkspace({ socketBase }: { socketBase: string }) {
	const [state, dispatch] = useReducer(sessionsReducer, null, () => restoreSessions(readStored()));

	useEffect(() => {
		try {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(state.sessions));
		} catch {
			/* the tabs just will not be remembered */
		}
	}, [state.sessions]);

	const open = () => dispatch({ type: "open" });

	return (
		<div className="flex min-h-0 flex-1 flex-col">
			<SessionTabs
				sessions={state.sessions}
				active={state.active}
				onSelect={(name) => dispatch({ type: "select", name })}
				onClose={(name) => dispatch({ type: "close", name })}
				onOpen={open}
			/>

			{state.sessions.length > 0 ? (
				<div className="relative min-h-0 flex-1 overflow-hidden rounded-card border border-border bg-terminal-background">
					{state.sessions.map((name) => (
						<TerminalPane
							key={name}
							name={name}
							socketBase={socketBase}
							visible={name === state.active}
							onEnded={() => dispatch({ type: "close", name })}
						/>
					))}
				</div>
			) : (
				<EmptyState
					icon={SquareTerminal}
					title="No sessions open"
					description="Sessions you closed are still running in the console; opening one by the same name reattaches."
					action={<Button type="button" icon={Plus} onClick={open}>New session</Button>}
				/>
			)}
		</div>
	);
}
