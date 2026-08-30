"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

import { Banner, Card } from "@/components/ui";
import { useJobStream } from "@/components/helm/use-job-stream";
import { describeJob } from "@/lib/helm/status";
import { JobBadge } from "./job-badge";
import { JobLog } from "./job-log";
import type { HelmJob } from "@/lib/api/types";

/**
 * The operation currently running against a release, and its output.
 *
 * This is what the 202 became. Before there was a Job behind it, the only way to
 * find out what a deploy had done was to re-read the release and infer — and the
 * reason a deploy failed lived in the API pod's own log, where an operator needed
 * kubectl to reach it.
 *
 * Rendered from a job the server found, so a page loaded by somebody who never
 * started the operation still shows it. That is also what makes a self-upgrade
 * legible: the panel goes away while its own pods are replaced, and the page that
 * comes back finds the job still running and picks it up.
 */
export function JobPanel({ job }: { job: HelmJob | null }) {
	const state = useJobStream(job?.name ?? null, job);
	const router = useRouter();
	const finished = state.terminal;

	// The release and the deployment record are what actually changed, and they
	// are rendered by the server. Refresh once when the operation ends rather
	// than polling throughout.
	useEffect(() => {
		if (finished) router.refresh();
	}, [finished, router]);

	if (!job) return null;

	return (
		<Card padding="sm" className="flex flex-col gap-3">
			<div className="flex flex-wrap items-center justify-between gap-2">
				<div className="flex items-center gap-2">
					<JobBadge phase={state.phase} />
					<span className="text-sm">
						<span className="text-muted-foreground">{job.operation} — </span>
						{describeJob(state.phase, state.reason)}
					</span>
				</div>
				<code className="truncate font-mono text-xs text-muted-foreground">{job.name}</code>
			</div>

			{/* A dropped stream is not a failed deploy, and saying so matters most
			    here: rolling out this panel's own chart replaces the pods serving
			    this page, so the connection drops every time and the operation
			    carries on regardless. */}
			{state.streamError && !finished && (
				<Banner
					variant="info"
					message={`Lost contact with the job — reconnecting. The operation is still running: ${state.streamError}`}
				/>
			)}

			<JobLog lines={state.lines} truncated={state.truncated} />
		</Card>
	);
}
