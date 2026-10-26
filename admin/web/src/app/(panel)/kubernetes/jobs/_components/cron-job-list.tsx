import { CalendarClock } from "lucide-react";
import { Td, Th } from "@/components/ui/table";
import { Directory } from "../../../_components/directory";
import { formatAge } from "@/lib/format/age";
import type { ClusterCronJob } from "@/lib/api/types";

/**
 * The schedules. A server component: nothing here is interactive.
 *
 * Last success sits beside last scheduled because the gap between the two is
 * the question this table answers — a schedule that fires and never succeeds
 * looks perfectly healthy from its cron line alone.
 */
export function CronJobList({
	cronJobs,
	error,
	showNamespace,
	now,
}: {
	cronJobs: ClusterCronJob[];
	error: string | null;
	showNamespace: boolean;
	now: Date;
}) {
	return (
		<section className="flex flex-col gap-3">
			<h2 className="text-lg font-medium">Cron jobs</h2>
			<Directory
				error={error}
				isEmpty={error === null && cronJobs.length === 0}
				minWidth={showNamespace ? "min-w-[860px]" : "min-w-[720px]"}
				empty={{ icon: CalendarClock, title: "No cron jobs", description: "Nothing here runs on a schedule." }}
				columns={
					<>
						{showNamespace ? <Th>Namespace</Th> : null}
						<Th>Name</Th>
						<Th>Schedule</Th>
						<Th className="w-px whitespace-nowrap text-right">Running</Th>
						<Th className="w-px whitespace-nowrap text-right">Last scheduled</Th>
						<Th className="w-px whitespace-nowrap text-right">Last success</Th>
					</>
				}
				rows={cronJobs.map((cronJob) => (
					<tr key={`${cronJob.namespace}/${cronJob.name}`}>
						{showNamespace ? <Td className="text-muted-foreground">{cronJob.namespace}</Td> : null}
						<Td className="font-medium">
							{cronJob.name}
							{cronJob.suspended ? <span className="ml-2 text-xs text-muted-foreground">suspended</span> : null}
						</Td>
						<Td className="font-mono text-xs text-muted-foreground">
							{cronJob.schedule}
							{cronJob.timeZone ? ` (${cronJob.timeZone})` : ""}
						</Td>
						<Td className="w-px whitespace-nowrap text-right text-muted-foreground">{cronJob.active}</Td>
						<Td className="w-px whitespace-nowrap text-right text-muted-foreground">
							{cronJob.lastScheduled ? formatAge(cronJob.lastScheduled, now) : "never"}
						</Td>
						<Td className="w-px whitespace-nowrap text-right text-muted-foreground">
							{cronJob.lastSucceeded ? formatAge(cronJob.lastSucceeded, now) : "never"}
						</Td>
					</tr>
				))}
			/>
		</section>
	);
}
