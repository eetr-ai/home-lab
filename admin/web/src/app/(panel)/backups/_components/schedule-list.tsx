"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { DatabaseBackup, Play, Plus, Trash2 } from "lucide-react";
import { deleteSchedule, runSchedule } from "@/app/actions/backups";
import { Button, IconButton, InlineDeleteConfirm, Td, Th } from "@/components/ui";
import { ActionsHeader, Directory } from "../../_components/directory";
import { InteractiveRow, stopRowActivation } from "../../_components/interactive-row";
import { useRowDelete } from "../../_components/use-row-delete";
import { describeCron } from "@/lib/backups/cron";
import { formatAge } from "@/lib/format/age";
import type { BackupDatabase, BackupSchedule, GoogleStatus } from "@/lib/api/types";
import { SchedulePanel } from "./schedule-panel";

export function ScheduleList({
	schedules,
	loadError,
	databases,
	status,
	now,
}: {
	schedules: BackupSchedule[];
	loadError: string | null;
	databases: BackupDatabase[];
	status: GoogleStatus | null;
	now: Date;
}) {
	const [error, setError] = useState<string | null>(null);
	const [creating, setCreating] = useState(false);
	// Per schedule: starting one run must not hold up the button of another.
	const [starting, setStarting] = useState<ReadonlySet<string>>(new Set());
	const rowDelete = useRowDelete(setError);
	const router = useRouter();

	// A schedule needs an OAuth client to upload with; without one the API refuses
	// to create it, so the button says so by being off rather than by failing.
	const create = (
		<Button icon={Plus} onClick={() => setCreating(true)} disabled={!status?.configured}>
			New schedule
		</Button>
	);

	async function runNow(name: string) {
		setError(null);
		setStarting((current) => new Set(current).add(name));
		const result = await runSchedule(name);
		setStarting((current) => {
			const next = new Set(current);
			next.delete(name);
			return next;
		});
		if (!result.ok) setError(result.error);
		else router.push(`/backups/${encodeURIComponent(name)}`);
	}

	return (
		<>
			<div className="flex items-center justify-between">
				<h2 className="text-lg font-medium">Schedules</h2>
				{create}
			</div>

			<Directory
				error={error ?? loadError}
				isEmpty={schedules.length === 0}
				minWidth="min-w-[860px]"
				empty={{
					icon: DatabaseBackup,
					title: "No backups scheduled",
					description:
						"Pick the databases, when to run, the Drive folder, and how many backups of each to keep.",
					action: create,
				}}
				columns={
					<>
						<Th>Schedule</Th>
						<Th>When</Th>
						<Th>Databases</Th>
						<Th>Folder</Th>
						<Th className="w-px whitespace-nowrap text-right">Keep</Th>
						<Th className="w-px whitespace-nowrap text-right">Last success</Th>
						<ActionsHeader />
					</>
				}
				rows={schedules.map((schedule) => (
					<InteractiveRow
						key={schedule.name}
						onActivate={() => router.push(`/backups/${encodeURIComponent(schedule.name)}`)}
					>
						<Td className="font-medium">
							<Link href={`/backups/${encodeURIComponent(schedule.name)}`} onClick={stopRowActivation}>
								{schedule.name}
							</Link>
							{schedule.suspended ? (
								<span className="ml-2 text-xs text-muted-foreground">suspended</span>
							) : null}
						</Td>
						<Td className="text-muted-foreground">{describeCron(schedule.cron, schedule.timeZone)}</Td>
						<Td className="max-w-[16rem] truncate text-muted-foreground" title={names(schedule)}>
							{names(schedule)}
						</Td>
						<Td className="text-muted-foreground">{schedule.folder.name}</Td>
						<Td className="w-px whitespace-nowrap text-right text-muted-foreground">{schedule.keep}</Td>
						<Td className="w-px whitespace-nowrap text-right text-muted-foreground">
							{/* A schedule that ran and never succeeded is the one to look at, so
							    "never" is said rather than left blank. */}
							{schedule.lastSucceeded ? formatAge(schedule.lastSucceeded, now) : "never"}
						</Td>
						<Td className="text-right" onClick={stopRowActivation}>
							{rowDelete.confirmingId === schedule.name ? (
								<InlineDeleteConfirm
									label="Delete this schedule? Its backups stay in Drive."
									busy={rowDelete.isDeleting(schedule.name)}
									onConfirm={() => rowDelete.confirm(schedule.name, () => deleteSchedule(schedule.name))}
									onCancel={rowDelete.cancel}
								/>
							) : (
								<div className="flex items-center justify-end gap-1">
									<IconButton
										type="button"
										aria-label={`Run ${schedule.name} now`}
										disabled={starting.has(schedule.name)}
										onClick={() => runNow(schedule.name)}
									>
										<Play className="h-4 w-4" />
									</IconButton>
									<IconButton
										type="button"
										variant="danger"
										aria-label={`Delete ${schedule.name}`}
										onClick={() => rowDelete.ask(schedule.name)}
									>
										<Trash2 className="h-4 w-4" />
									</IconButton>
								</div>
							)}
						</Td>
					</InteractiveRow>
				))}
			/>

			<SchedulePanel
				open={creating}
				databases={databases}
				status={status}
				onClose={() => setCreating(false)}
			/>
		</>
	);
}

function names(schedule: BackupSchedule): string {
	return schedule.databases.map((database) => `${database.engine}/${database.name}`).join(", ");
}
