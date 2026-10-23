"use client";

import { useEffect, useReducer } from "react";
import { Plus, SquareTerminal, X } from "lucide-react";
import { Button, EmptyState, IconButton, cn } from "@/components/ui";
import { restoreSessions, sessionsReducer, STORAGE_KEY } from "@/lib/console/sessions";
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
			<div className="mb-3 flex flex-wrap items-center gap-1 border-b border-border" role="tablist" aria-label="Sessions">
				{state.sessions.map((name) => {
					const active = name === state.active;
					return (
						<div
							key={name}
							className={cn(
								"-mb-px flex items-center gap-1 border-b-2 pl-3 pr-1",
								active ? "border-brand text-foreground" : "border-transparent text-muted-foreground",
							)}
						>
							<button
								type="button"
								role="tab"
								aria-selected={active}
								onClick={() => dispatch({ type: "select", name })}
								className="py-2 font-mono text-sm hover:text-foreground"
							>
								{name}
							</button>
							<IconButton
								type="button"
								aria-label={`Close ${name} (the session keeps running)`}
								title="Close the tab; the session keeps running"
								onClick={() => dispatch({ type: "close", name })}
							>
								<X className="h-3.5 w-3.5" />
							</IconButton>
						</div>
					);
				})}
				<IconButton type="button" aria-label="New session" title="New session" onClick={open} className="ml-1">
					<Plus className="h-4 w-4" />
				</IconButton>
			</div>

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
