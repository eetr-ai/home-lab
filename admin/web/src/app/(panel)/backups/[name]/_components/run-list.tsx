"use client";

import { useState } from "react";
import { History, ScrollText } from "lucide-react";
import { IconButton, Td, Th } from "@/components/ui";
import { cn } from "@/components/ui/cn";
import { ActionsHeader, Directory } from "../../../_components/directory";
import { LogPanel } from "../../../_components/logs/log-panel";
import { formatAge } from "@/lib/format/age";
import type { BackupRun, BackupRunPhase } from "@/lib/api/types";

const PHASE_TONE: Record<BackupRunPhase, string> = {
	pending: "border-border-strong bg-surface-sunken text-muted-foreground",
	running: "border-accent-fg/30 bg-accent-bg text-accent-fg",
	succeeded: "border-success-border bg-success-bg text-success-fg",
	failed: "border-danger-border bg-danger-bg text-danger-fg",
};

/**
 * A schedule's recent runs, and the log of whichever one is open.
 *
 * The log is the existing pod log panel, opened on the dump and upload
 * containers in the order they ran — a run explains itself there, not here.
 */
export function RunList({
	runs,
	loadError,
	now,
}: {
	runs: BackupRun[];
	loadError: string | null;
	now: Date;
}) {
	const [open, setOpen] = useState<BackupRun | null>(null);

	return (
		<>
			<h2 className="text-lg font-medium">Runs</h2>
			<Directory
				error={loadError}
				// Empty only after a successful load: a failed one is not "none".
				isEmpty={loadError === null && runs.length === 0}
				minWidth="min-w-[640px]"
				empty={{ icon: History, title: "No runs yet", description: "Run it now, or wait for the clock." }}
				columns={
					<>
						<Th>Started</Th>
						<Th>Outcome</Th>
						<Th>How</Th>
						<ActionsHeader />
					</>
				}
				rows={runs.map((run) => (
					<tr key={run.name}>
						<Td className="text-muted-foreground">{formatAge(run.startedAt ?? run.createdAt, now)} ago</Td>
						<Td>
							<span className={cn("rounded-chip border px-2 py-0.5 text-xs", PHASE_TONE[run.phase])}>
								{run.phase}
								{run.reason ? ` — ${run.reason}` : ""}
							</span>
						</Td>
						<Td className="text-muted-foreground">{run.manual ? "Run now" : "Scheduled"}</Td>
						<Td className="text-right">
							<IconButton
								type="button"
								aria-label={`Show the log of ${run.name}`}
								disabled={!run.pod}
								onClick={() => setOpen(run)}
							>
								<ScrollText className="h-4 w-4" />
							</IconButton>
						</Td>
					</tr>
				))}
			/>
			{open?.pod ? (
				<LogPanel
					key={open.pod}
					namespace={open.namespace}
					pod={open.pod}
					containers={open.containers}
					onClose={() => setOpen(null)}
				/>
			) : null}
		</>
	);
}
