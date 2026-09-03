"use client";

import { PencilLine, Play } from "lucide-react";
import { Button } from "@/components/ui";

/**
 * The console's action row: a context line, and the two ways to run what is in
 * the editor. Run is the read-only path; Execute commits, so it stops for an
 * inline confirmation first — and while it is confirming, it is the only thing
 * offered, so there is one decision to make.
 */
export function ConsoleToolbar({
	context,
	canRun,
	running,
	executing,
	confirming,
	database,
	onRun,
	onExecute,
	onConfirmExecute,
	onCancelExecute,
}: {
	context: string;
	canRun: boolean;
	running: boolean;
	executing: boolean;
	confirming: boolean;
	database: string;
	onRun: () => void;
	onExecute: () => void;
	onConfirmExecute: () => void;
	onCancelExecute: () => void;
}) {
	return (
		<div className="flex flex-wrap items-center gap-3">
			<span className="text-sm text-muted-foreground">{context}</span>
			{confirming ? (
				<div className="ml-auto flex flex-wrap items-center gap-2">
					<span className="text-sm text-warning-fg">
						Commit to {database}? This cannot be undone from here.
					</span>
					<Button variant="secondary" onClick={onCancelExecute} disabled={executing}>
						Cancel
					</Button>
					<Button variant="destructiveConfirm" loading={executing} onClick={onConfirmExecute}>
						Execute
					</Button>
				</div>
			) : (
				<div className="ml-auto flex items-center gap-2">
					<Button variant="secondary" icon={PencilLine} disabled={!canRun} onClick={onExecute}>
						Execute
					</Button>
					<Button icon={Play} loading={running} disabled={!canRun} onClick={onRun}>
						Run
					</Button>
				</div>
			)}
		</div>
	);
}
