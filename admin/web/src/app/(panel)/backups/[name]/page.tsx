import { notFound } from "next/navigation";
import { googleStatus, listFiles, listRuns, readSchedule } from "@/app/actions/backups";
import { listDatabases as listPostgres } from "@/app/actions/postgres";
import { listDatabases as listMongo } from "@/app/actions/mongo";
import { BackLink } from "../../_components/back-link";
import { availableDatabases } from "../_components/available-databases";
import { ScheduleDetail } from "./_components/schedule-detail";

export const dynamic = "force-dynamic";

/**
 * One schedule: what it does, its recent runs with their logs, and what it has
 * kept in Drive.
 *
 * The runs and the files fail separately and say so separately. "Drive did not
 * answer" must not read as "there are no backups", and a missing schedule is a
 * 404 only when the API said not found — any other failure is shown as one.
 */
export default async function BackupSchedulePage({ params }: { params: Promise<{ name: string }> }) {
	const { name } = await params;
	const schedule = await readSchedule(decodeURIComponent(name));
	if (!schedule.ok && /not found|not_found/i.test(schedule.error)) notFound();

	const [runs, files, status, postgres, mongo] = schedule.ok
		? await Promise.all([
				listRuns(schedule.data.name),
				listFiles(schedule.data.name),
				googleStatus(),
				listPostgres(),
				listMongo(),
			])
		: [null, null, null, null, null];

	return (
		<div className="flex flex-col gap-6">
			<BackLink href="/backups" label="All schedules" />
			{schedule.ok && runs && files && status && postgres && mongo ? (
				<ScheduleDetail
					schedule={schedule.data}
					runs={runs.ok ? runs.data : []}
					runsError={runs.ok ? null : runs.error}
					files={files.ok ? files.data : []}
					filesError={files.ok ? null : files.error}
					status={status.ok ? status.data : null}
					databases={availableDatabases(postgres, mongo)}
					now={new Date()}
				/>
			) : (
				<p className="text-sm text-danger-fg">{schedule.ok ? "" : schedule.error}</p>
			)}
		</div>
	);
}
