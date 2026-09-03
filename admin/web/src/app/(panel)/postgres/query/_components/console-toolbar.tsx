"use client";

import { useEffect } from "react";
import { Play } from "lucide-react";
import { Button } from "@/components/ui";

/**
 * The console's action row: a context line and one Run button. Run reads or
 * writes depending on the statement (see classifyStatement) — a read runs at
 * once, and a write stops here for an inline confirmation first, because a commit
 * cannot be taken back. While it is confirming, the confirmation is the only
 * thing offered, so there is one decision to make.
 */
export function ConsoleToolbar({
	context,
	canRun,
	running,
	executing,
	confirming,
	database,
	onRun,
	onConfirm,
	onCancel,
}: {
	context: string;
	canRun: boolean;
	running: boolean;
	executing: boolean;
	confirming: boolean;
	database: string;
	onRun: () => void;
	onConfirm: () => void;
	onCancel: () => void;
}) {
	// Escape cancels the confirmation from anywhere; Enter is handled by focusing
	// the Commit button below, so it commits without the operator reaching for the
	// mouse. Only while confirming, and not while the commit is already running.
	useEffect(() => {
		if (!confirming || executing) return;
		function onKeyDown(event: KeyboardEvent) {
			if (event.key === "Escape") {
				event.preventDefault();
				onCancel();
			}
		}
		document.addEventListener("keydown", onKeyDown);
		return () => document.removeEventListener("keydown", onKeyDown);
	}, [confirming, executing, onCancel]);

	return (
		<div className="flex flex-wrap items-center gap-3">
			<span className="text-sm text-muted-foreground">{context}</span>
			{confirming ? (
				<div className="ml-auto flex flex-wrap items-center gap-2">
					<span className="text-sm text-warning-fg">
						This modifies {database} and commits. Continue?
					</span>
					<Button variant="secondary" onClick={onCancel} disabled={executing}>
						Cancel
					</Button>
					{/* autoFocus so a plain Enter commits, no mouse needed. */}
					<Button variant="destructiveConfirm" autoFocus loading={executing} onClick={onConfirm}>
						Commit
					</Button>
				</div>
			) : (
				<Button
					className="ml-auto"
					icon={Play}
					loading={running || executing}
					disabled={!canRun}
					onClick={onRun}
				>
					Run
				</Button>
			)}
		</div>
	);
}
