"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { CalendarClock, Pencil, Play } from "lucide-react";
import { runSchedule } from "@/app/actions/backups";
import { Banner, Button, SectionCard } from "@/components/ui";
import { describeCron } from "@/lib/backups/cron";
import { formatAge } from "@/lib/format/age";
import type { BackupDatabase, BackupFile, BackupRun, BackupSchedule, GoogleStatus } from "@/lib/api/types";
import { isActive } from "@/lib/backups/schedule-events";
import { SchedulePanel } from "../../_components/schedule-panel";
import { useScheduleStream } from "./use-schedule-stream";
import { RunList } from "./run-list";
import { FileList } from "./file-list";

export function ScheduleDetail({
	schedule: initialSchedule,
	runs: initialRuns,
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
	// The run just started, until the stream has seen it: between the click and
	// the next reading the run exists and the page does not know it yet, and the
	// button must not come back in that gap.
	const [awaiting, setAwaiting] = useState<string | null>(null);
	const router = useRouter();

	// A finished run is the moment what is kept in Drive changes, and the stream
	// does not carry the files, so the page re-reads them then.
	const live = useScheduleStream({ schedule: initialSchedule, runs: initialRuns }, now, () =>
		router.refresh(),
	);
	const { schedule, runs } = live.state;
	const busy = starting || runs.some(isActive) || (awaiting !== null && !runs.some((run) => run.name === awaiting));

	async function runNow() {
		setError(null);
		setStarting(true);
		try {
			const result = await runSchedule(schedule.name);
			if (!result.ok) setError(result.error);
			else setAwaiting(result.data.name);
		} catch {
			setError("the panel could not reach its server; try again");
		} finally {
			setStarting(false);
		}
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
					<dd>{schedule.lastSucceeded ? formatAge(schedule.lastSucceeded, live.at) + " ago" : "never"}</dd>
				</dl>
				<div className="mt-4 flex gap-2">
					<Button icon={Play} loading={busy} onClick={runNow}>
						{busy ? "Running…" : "Run now"}
					</Button>
					<Button variant="secondary" icon={Pencil} onClick={() => setEditing(true)}>
						Edit
					</Button>
				</div>
			</SectionCard>

			<RunList runs={runs} loadError={runsError} now={live.at} />
			<FileList schedule={schedule.name} files={files} loadError={filesError} busy={busy} now={now} />

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
