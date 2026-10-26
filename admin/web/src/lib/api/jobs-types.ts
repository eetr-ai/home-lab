/**
 * The Jobs tab's wire types, mirroring admin/api/internal/kube/jobs.go.
 *
 * Their own module because cluster-types.ts is at the length the lint allows.
 * Import them from `@/lib/api/types`, which re-exports these.
 */

/** A schedule that creates Jobs. */
export interface ClusterCronJob {
	namespace: string;
	name: string;
	schedule: string;
	/** Empty when the controller's own zone applies. */
	timeZone?: string;
	suspended: boolean;
	/** How many of its Jobs are running now. */
	active: number;
	lastScheduled?: string;
	lastSucceeded?: string;
	createdAt: string;
}

export type ClusterJobStatus = "pending" | "running" | "succeeded" | "failed";

/** One run of something to completion. */
export interface ClusterJob {
	namespace: string;
	name: string;
	/** The schedule that made it, when one did. */
	cronJob?: string;
	status: ClusterJobStatus;
	reason?: string;
	succeeded: number;
	/** Absent for a work-queue Job, which has no target count. */
	completions?: number;
	startedAt?: string;
	finishedAt?: string;
	createdAt: string;
}

/** GET /api/kubernetes/jobs, and per namespace. */
export interface JobListing {
	cronJobs: ClusterCronJob[];
	jobs: ClusterJob[];
}
