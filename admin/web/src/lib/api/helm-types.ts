/**
 * The Helm wire types, mirroring admin/api/internal/helm/types.go.
 *
 * Their own module rather than an addition to types.ts, which is already at the
 * length the lint allows. Import either from `@/lib/api/types`, which re-exports
 * these.
 */

export interface HelmRelease {
	name: string;
	namespace: string;
	/** Counts up and never repeats: rolling back creates a new revision. */
	revision: number;
	/** Helm's own vocabulary — deployed, failed, pending-upgrade, superseded. */
	status: string;
	chart: string;
	chartVersion: string;
	appVersion: string;
	/** "Upgrade complete", or the reason one failed. */
	description?: string;
	/** RFC 3339. */
	updatedAt: string;
}

export interface HelmReleaseDetail extends HelmRelease {
	/** What the release was configured with, not the chart's defaults merged in. */
	values: Record<string, unknown>;
	notes?: string;
}

export interface HelmRevision {
	revision: number;
	status: string;
	chartVersion: string;
	appVersion: string;
	description?: string;
	updatedAt: string;
}

export interface HelmChartVersion {
	version: string;
	appVersion?: string;
}

/** A chart this lab has declared for a namespace. Desired state, nothing more. */
export interface HelmDeployment {
	id: string;
	namespace: string;
	releaseName: string;
	/** oci://host/path/chart, or an https chart repository. Never has credentials. */
	chartRef: string;
	createdBy: string;
	/** RFC 3339. */
	createdAt: string;
}

/**
 * One (chart version, values) pair that was declared.
 *
 * Append-only and numbered from 1. A version with no `rolledOutAt` was written
 * and never applied, which is what makes editing values and deploying them two
 * separate decisions.
 */
export interface HelmDeploymentVersion {
	version: number;
	chartVersion: string;
	/** The document as it was written, comments and all. */
	valuesYaml: string;
	source: "panel" | "ci";
	createdBy: string;
	createdAt: string;
	rolledOutAt?: string;
	helmRevision?: number;
}

/**
 * How a record stands against the cluster.
 *
 * The whole reason for holding one: the two can disagree, and the panel says so
 * rather than picking a side. `unknown` means the live release could not be read
 * at all — deliberately distinct from `not-installed`, which would invite
 * installing a second copy of something already running.
 */
export type HelmDeploymentState =
	| "in-sync"
	| "pending"
	| "drifted"
	| "not-installed"
	| "unknown";

export interface HelmDeploymentSummary extends HelmDeployment {
	current: HelmDeploymentVersion;
	/** Helm's word for the live release, absent when there is none. */
	status?: string;
	state: HelmDeploymentState;
}

export interface HelmDeploymentDetail extends HelmDeploymentSummary {
	release?: HelmReleaseDetail;
	/** Why the live release could not be read, when it could not. */
	releaseError?: string;
	/** Newest first. */
	versions: HelmDeploymentVersion[];
}

/** Declaring a deployment. Writes a record; puts nothing on the cluster. */
export interface DeclareDeployment {
	namespace: string;
	name: string;
	chartRef: string;
	version: string;
	valuesYaml: string;
}

/** Adding a version without rolling it out. */
export interface AddDeploymentVersion {
	version: string;
	valuesYaml: string;
}

/** Applying a declared version. An absent `version` means the newest. */
export interface RolloutDeployment {
	version?: number;
	rollbackOnFailure?: boolean;
}

/**
 * What a mutation answers with.
 *
 * The operation was accepted, not performed: Helm waits for pods, which outlasts
 * every timeout between here and the API.
 */
export interface HelmAccepted {
	namespace: string;
	release: string;
	/** rollout, rollback, or uninstall. */
	operation: string;
	/**
	 * The Kubernetes Job doing the work, in the panel's own namespace.
	 *
	 * This is what makes the 202 followable: the job has a status and a log, so
	 * "did it work" is a question with a direct answer rather than one inferred by
	 * reading the release back.
	 */
	job: string;
	message: string;
}

/** How far along a Helm operation is. */
export type HelmJobPhase = "pending" | "running" | "succeeded" | "failed";

/**
 * One Helm operation, as the Kubernetes Job performing it.
 *
 * Nothing here is stored anywhere: every field is read off the Job or derived
 * from its labels. Finished jobs are removed by the cluster after a TTL, so this
 * is a live view rather than a history — what survives a deploy is the release
 * and its deployment record.
 */
export interface HelmJob {
	name: string;
	/** What the operation targets, not where the job runs. */
	namespace: string;
	release: string;
	operation: string;
	deploymentId?: string;
	phase: HelmJobPhase;
	/** Why it failed, when Kubernetes named a reason. Helm's own account is in the log. */
	reason?: string;
	actor?: string;
	/** Empty until a pod has been scheduled, which is not immediate. */
	pod?: string;
	createdAt: string;
	startedAt?: string;
	finishedAt?: string;
}
