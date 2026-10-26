import { ListChecks } from "lucide-react";
import { Td, Th } from "@/components/ui/table";
import { cn } from "@/components/ui/cn";
import { Directory } from "../../../_components/directory";
import { formatAge } from "@/lib/format/age";
import type { ClusterJob, ClusterJobStatus } from "@/lib/api/types";

const STATUS_TONE: Record<ClusterJobStatus, string> = {
	pending: "border-border-strong bg-surface-sunken text-muted-foreground",
	running: "border-accent-fg/30 bg-accent-bg text-accent-fg",
	succeeded: "border-success-border bg-success-bg text-success-fg",
	failed: "border-danger-border bg-danger-bg text-danger-fg",
};

/**
 * The runs, newest first within each namespace. A Job's pod and log are on the
 * Pods tab, named after the Job, which is where a failed one explains itself.
 */
export function JobList({
	jobs,
	error,
	loaded,
	showNamespace,
	now,
}: {
	jobs: ClusterJob[];
	error: string | null;
	/** False when the listing failed; the page shows that error once, elsewhere. */
	loaded: boolean;
	showNamespace: boolean;
	now: Date;
}) {
	return (
		<section className="flex flex-col gap-3">
			<h2 className="text-lg font-medium">Jobs</h2>
			<Directory
				error={error}
				isEmpty={loaded && jobs.length === 0}
				minWidth={showNamespace ? "min-w-[860px]" : "min-w-[720px]"}
				empty={{ icon: ListChecks, title: "No jobs", description: "Finished jobs are removed by their TTL or their schedule's history limit." }}
				columns={
					<>
						{showNamespace ? <Th>Namespace</Th> : null}
						<Th>Name</Th>
						<Th>Made by</Th>
						<Th>Status</Th>
						<Th className="w-px whitespace-nowrap text-right">Completions</Th>
						<Th className="w-px whitespace-nowrap text-right">Started</Th>
					</>
				}
				rows={jobs.map((job) => (
					<tr key={`${job.namespace}/${job.name}`}>
						{showNamespace ? <Td className="text-muted-foreground">{job.namespace}</Td> : null}
						<Td className="font-medium">{job.name}</Td>
						<Td className="text-muted-foreground">{job.cronJob ?? "—"}</Td>
						<Td>
							<span className={cn("rounded border px-2 py-0.5 text-xs", STATUS_TONE[job.status])}>
								{job.status}
								{job.reason ? ` — ${job.reason}` : ""}
							</span>
						</Td>
						<Td className="w-px whitespace-nowrap text-right text-muted-foreground">
							{job.succeeded}/{job.completions}
						</Td>
						<Td className="w-px whitespace-nowrap text-right text-muted-foreground">
							{formatAge(job.startedAt ?? job.createdAt, now)}
						</Td>
					</tr>
				))}
			/>
		</section>
	);
}
