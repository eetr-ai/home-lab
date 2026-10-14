"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { CircleCheck, CircleX, Clock, Loader2, type LucideIcon } from "lucide-react";

import { Banner, buttonVariants, cn, Popover } from "@/components/ui";
import { useJobStream } from "@/components/helm/use-job-stream";
import { describeJob, jobIndicator, type Tone } from "@/lib/helm/status";
import { JobBadge } from "./job-badge";
import { JobLog } from "./job-log";
import type { HelmJob, HelmJobPhase } from "@/lib/api/types";

const ICONS: Record<HelmJobPhase, LucideIcon> = {
	pending: Clock,
	running: Loader2,
	succeeded: CircleCheck,
	failed: CircleX,
};

const ICON_TONE: Record<Tone, string> = {
	success: "text-success-fg",
	danger: "text-danger-fg",
	warning: "text-warning-fg",
	muted: "text-muted-foreground",
};

/**
 * The operation currently running against a release, as a button in the page's
 * header row, with its output one click away.
 *
 * It used to be a card above everything else on the page. The button says what
 * the card's first line said — and animates while something is happening — and
 * the popover is the rest of it: Helm's own account of the operation, as it
 * arrives.
 *
 * Mounted for as long as there is a job, whether or not the popover is open. It
 * owns the stream, and it is what refreshes the page when the operation ends, so
 * neither can depend on somebody having opened it.
 *
 * Rendered from a job the server found, so a page loaded by somebody who never
 * started the operation still shows it. That is also what makes a self-upgrade
 * legible: the button goes away while its own pods are replaced, and the page
 * that comes back finds the job still running and picks it up.
 */
export function JobStatus({ job }: { job: HelmJob | null }) {
	const state = useJobStream(job?.name ?? null, job);
	const router = useRouter();
	const [open, setOpen] = useState(false);
	const trigger = useRef<HTMLButtonElement>(null);
	const finished = state.terminal;

	// The release and the deployment record are what actually changed, and they
	// are rendered by the server. Refresh once when the operation ends rather
	// than polling throughout.
	useEffect(() => {
		if (finished) router.refresh();
	}, [finished, router]);

	if (!job) return null;

	const indicator = jobIndicator(state.phase, state.streamError);
	const Icon = ICONS[state.phase];

	return (
		<>
			<button
				ref={trigger}
				type="button"
				onClick={() => setOpen((wasOpen) => !wasOpen)}
				aria-haspopup="dialog"
				aria-expanded={open}
				aria-label={`${job.operation}: ${describeJob(state.phase, state.reason)}. Show the log`}
				className={cn(buttonVariants.secondary, "relative flex items-center gap-2")}
			>
				<Icon
					className={cn(
						"h-4 w-4 shrink-0",
						ICON_TONE[indicator.tone],
						state.phase === "running" && "motion-safe:animate-spin",
					)}
					aria-hidden
				/>
				<span>
					<span className="text-muted-foreground">{job.operation} · </span>
					{indicator.label}
				</span>
				{/* The corner dot is the "something is happening" signal that still
				    reads with reduced motion, where the ping is dropped and the dot
				    stays. Warning-toned when the stream has dropped mid-operation. */}
				{indicator.active ? (
					<span className="absolute -top-0.5 -right-0.5 flex h-2.5 w-2.5" aria-hidden>
						<span
							className={cn(
								"absolute inline-flex h-full w-full rounded-full opacity-75 motion-safe:animate-ping",
								indicator.lost ? "bg-warning-fg" : "bg-brand",
							)}
						/>
						<span
							className={cn(
								"relative inline-flex h-2.5 w-2.5 rounded-full",
								indicator.lost ? "bg-warning-fg" : "bg-brand",
							)}
						/>
					</span>
				) : null}
			</button>

			<Popover
				open={open}
				onRequestClose={() => setOpen(false)}
				anchor={trigger}
				title={`${job.operation} job`}
				width="lg"
			>
				<div className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-3">
					<div className="flex items-center gap-2">
						<JobBadge phase={state.phase} />
						<span className="text-sm">
							<span className="text-muted-foreground">{job.operation} — </span>
							{describeJob(state.phase, state.reason)}
						</span>
					</div>
					<code className="truncate font-mono text-xs text-muted-foreground">{job.name}</code>
				</div>

				<div className="flex flex-col gap-3 p-4">
					{/* A dropped stream is not a failed deploy, and saying so matters
					    most here: rolling out this panel's own chart replaces the pods
					    serving this page, so the connection drops every time and the
					    operation carries on regardless. */}
					{indicator.lost && (
						<Banner
							variant="info"
							message={`Lost contact with the job — reconnecting. The operation is still running: ${state.streamError}`}
						/>
					)}

					{state.lines.length === 0 ? (
						<p className="text-xs text-muted-foreground">No output yet.</p>
					) : (
						<JobLog lines={state.lines} truncated={state.truncated} />
					)}
				</div>
			</Popover>
		</>
	);
}
