"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { CalendarClock, Pencil, Play } from "lucide-react";
import { runSchedule } from "@/app/actions/backups";
import { Banner, Button, SectionCard } from "@/components/ui";
import { describeCron } from "@/lib/backups/cron";
import { formatAge } from "@/lib/format/age";
import type { BackupDatabase, BackupFile, BackupRun, BackupSchedule, GoogleStatus } from "@/lib/api/types";
import { SchedulePanel } from "../../_components/schedule-panel";
import { RunList } from "./run-list";
import { FileList } from "./file-list";

export function ScheduleDetail({
	schedule,
	runs,
	runsError,
	files,
	filesError,
	status,
	databases,
	now,
}: {
	schedule: BackupSchedule;
	runs: BackupRun[];
	runsError: string | null;
	files: BackupFile[];
	filesError: string | null;
	status: GoogleStatus | null;
	databases: BackupDatabase[];
	now: Date;
}) {
	const [editing, setEditing] = useState(false);
	const [error, setError] = useState<string | null>(null);
	const [starting, setStarting] = useState(false);
	const router = useRouter();

	async function runNow() {
		setError(null);
		setStarting(true);
		const result = await runSchedule(schedule.name);
		setStarting(false);
		if (!result.ok) setError(result.error);
		else router.refresh();
	}

	return (
		<>
			<SectionCard title={schedule.name} icon={CalendarClock}>
				<Banner variant="error" message={error} className="mb-4" />
				<dl className="grid grid-cols-[max-content_1fr] gap-x-6 gap-y-2 text-sm">
					<dt className="text-muted-foreground">When</dt>
					<dd>
						{describeCron(schedule.cron, schedule.timeZone)}
						{schedule.suspended ? " — suspended" : ""}
					</dd>
					<dt className="text-muted-foreground">Databases</dt>
					<dd>{schedule.databases.map((one) => `${one.engine}/${one.name}`).join(", ")}</dd>
					<dt className="text-muted-foreground">Folder</dt>
					<dd>{schedule.folder.name}</dd>
					<dt className="text-muted-foreground">Keeps</dt>
					<dd>The newest {schedule.keep} of each database</dd>
					<dt className="text-muted-foreground">Last success</dt>
					<dd>{schedule.lastSucceeded ? formatAge(schedule.lastSucceeded, now) + " ago" : "never"}</dd>
				</dl>
				<div className="mt-4 flex gap-2">
					<Button icon={Play} disabled={starting || schedule.running > 0} onClick={runNow}>
						{schedule.running > 0 ? "Running…" : "Run now"}
					</Button>
					<Button variant="secondary" icon={Pencil} onClick={() => setEditing(true)}>
						Edit
					</Button>
				</div>
			</SectionCard>

			<RunList runs={runs} loadError={runsError} now={now} />
			<FileList files={files} loadError={filesError} now={now} />

			<SchedulePanel
				open={editing}
				schedule={schedule}
				databases={databases}
				status={status}
				onClose={() => setEditing(false)}
			/>
		</>
	);
}
