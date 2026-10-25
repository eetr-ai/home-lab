import { googleStatus, listSchedules } from "@/app/actions/backups";
import { listDatabases as listPostgres } from "@/app/actions/postgres";
import { listDatabases as listMongo } from "@/app/actions/mongo";
import { DriveCard } from "./_components/drive-card";
import { ScheduleList } from "./_components/schedule-list";
import { availableDatabases } from "./_components/available-databases";

export const dynamic = "force-dynamic";

/**
 * The connection and the schedules.
 *
 * The database lists come along because creating a schedule means choosing from
 * them. A server the lab does not manage answers with a failure, and that is
 * simply an engine with nothing to offer — the API refuses a schedule naming it
 * anyway — so those failures are not banners here.
 */
export default async function BackupsPage({
	searchParams,
}: {
	searchParams: Promise<{ drive?: string; driveError?: string }>;
}) {
	const { drive, driveError } = await searchParams;
	const [status, schedules, postgres, mongo] = await Promise.all([
		googleStatus(),
		listSchedules(),
		listPostgres(),
		listMongo(),
	]);

	return (
		<div className="flex flex-col gap-6">
			<DriveCard
				status={status.ok ? status.data : null}
				loadError={status.ok ? null : status.error}
				justConnected={drive === "connected"}
				connectError={driveError ?? null}
			/>
			<ScheduleList
				schedules={schedules.ok ? schedules.data : []}
				loadError={schedules.ok ? null : schedules.error}
				databases={availableDatabases(postgres, mongo)}
				status={status.ok ? status.data : null}
				now={new Date()}
			/>
		</div>
	);
}
