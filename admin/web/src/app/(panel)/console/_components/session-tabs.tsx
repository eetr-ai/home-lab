"use client";

import type { KeyboardEvent } from "react";
import { Plus, X } from "lucide-react";
import { IconButton, cn } from "@/components/ui";
import { tabIds, tabKeyTarget } from "@/lib/console/sessions";

interface SessionTabsProps {
	sessions: string[];
	active: string | null;
	onSelect: (name: string) => void;
	onClose: (name: string) => void;
	onOpen: () => void;
}

/**
 * The session tabs: local state rather than route segments, so — per the UX
 * guidelines — a real ARIA tablist. Only tabs live inside it, focus roves (one tab
 * stop, arrows to move, Home and End), and each tab names the panel it controls.
 *
 * Closing is the Delete key on a focused tab. The × beside each one is the same
 * action for a pointer, kept out of the accessibility tree and the tab order so the
 * list holds nothing but tabs; "New session" sits outside the list for that reason.
 */
export function SessionTabs({ sessions, active, onSelect, onClose, onOpen }: SessionTabsProps) {
	const focusTab = (name: string) => document.getElementById(tabIds(name).tab)?.focus();

	const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>, name: string) => {
		if (event.key === "Delete" || event.key === "Backspace") {
			event.preventDefault();
			const rest = sessions.filter((n) => n !== name);
			onClose(name);
			const next = rest[Math.min(sessions.indexOf(name), rest.length - 1)];
			if (next) requestAnimationFrame(() => focusTab(next));
			return;
		}
		const target = tabKeyTarget(sessions, name, event.key);
		if (!target) return;
		event.preventDefault();
		onSelect(target);
		focusTab(target);
	};

	return (
		<div className="mb-3 flex flex-wrap items-center gap-1 border-b border-border">
			<div role="tablist" aria-label="Sessions" className="flex flex-wrap gap-1">
				{sessions.map((name) => {
					const selected = name === active;
					const ids = tabIds(name);
					return (
						<div
							key={name}
							role="presentation"
							className={cn(
								"-mb-px flex items-center gap-1 border-b-2 pl-3 pr-1",
								selected ? "border-brand text-foreground" : "border-transparent text-muted-foreground",
							)}
						>
							<button
								type="button"
								role="tab"
								id={ids.tab}
								aria-controls={ids.panel}
								aria-selected={selected}
								aria-keyshortcuts="Delete"
								tabIndex={selected ? 0 : -1}
								onClick={() => onSelect(name)}
								onKeyDown={(event) => onKeyDown(event, name)}
								className="py-2 font-mono text-sm hover:text-foreground"
							>
								{name}
							</button>
							<span
								aria-hidden="true"
								title="Close the tab; the session keeps running"
								onClick={() => onClose(name)}
								className="cursor-pointer rounded-full p-1.5 text-muted-foreground hover:bg-surface-hover hover:text-foreground"
							>
								<X className="h-3.5 w-3.5" />
							</span>
						</div>
					);
				})}
			</div>
			<IconButton type="button" aria-label="New session" title="New session" onClick={onOpen} className="ml-1">
				<Plus className="h-4 w-4" />
			</IconButton>
		</div>
	);
}
