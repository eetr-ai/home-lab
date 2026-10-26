import { listJobs } from "@/app/actions/kube";
import { readNamespaceScope } from "../../_components/namespace-scope-server";
import { isAllNamespaces } from "@/lib/kube/scope";
import { CronJobList } from "./_components/cron-job-list";
import { JobList } from "./_components/job-list";

export const dynamic = "force-dynamic";

/**
 * The CronJobs and the Jobs in the chosen namespace, or in every namespace.
 *
 * One request for both: a Job is most often read beside the schedule that made
 * it, and the API answers them together for that reason. Read-only — what
 * schedules exist is somebody's chart or, for backups, the Backups page.
 */
export default async function JobsPage({
	searchParams,
}: {
	searchParams: Promise<{ namespace?: string }>;
}) {
	const { namespace } = await searchParams;
	const scope = await readNamespaceScope(namespace);
	const listing = await listJobs(scope);
	const error = listing.ok ? null : listing.error;
	const showNamespace = isAllNamespaces(scope);
	const now = new Date();

	return (
		<div className="flex flex-col gap-6">
			<CronJobList
				cronJobs={listing.ok ? listing.data.cronJobs : []}
				error={error}
				showNamespace={showNamespace}
				now={now}
			/>
			{/* The error once, above; the runs list does not repeat it. */}
			<JobList jobs={listing.ok ? listing.data.jobs : []} error={null} loaded={listing.ok} showNamespace={showNamespace} now={now} />
		</div>
	);
}
