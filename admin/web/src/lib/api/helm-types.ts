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

export interface HelmChart {
	/** The catalog key a request uses. Need not be the chart's own name. */
	name: string;
	chart: string;
	repository: string;
	description?: string;
	/** The versions this lab permits, when it pins any. */
	versions?: string[];
}

/**
 * A catalogue entry with what can actually be installed.
 *
 * Not an extension of HelmChart: `versions` means something different here. On
 * the entry it is what this lab pins; here it is the intersection of that with
 * what the repository offers, which is the list to render.
 */
export interface HelmChartListing {
	name: string;
	chart: string;
	repository: string;
	description?: string;
	versions: HelmChartVersion[];
	/** When the version list was read. Absent when the repository was unreachable. */
	fetchedAt?: string;
	/** The repository could not be reached; versions is what configuration knew. */
	unavailable?: boolean;
}

/** POST /api/helm/namespaces/{namespace}/releases */
export interface InstallRelease {
	name: string;
	chart: string;
	/** Exact. A range or "latest" is refused by the API rather than resolved. */
	version: string;
	values?: Record<string, unknown>;
	rollbackOnFailure?: boolean;
}

/**
 * PUT /api/helm/namespaces/{namespace}/releases/{release}
 *
 * Omitting values means the release keeps its own, which is the normal case.
 */
export interface UpgradeRelease {
	version: string;
	values?: Record<string, unknown>;
	rollbackOnFailure?: boolean;
}

/** What every Helm mutation answers with. The work is accepted, not done. */
export interface HelmAccepted {
	namespace: string;
	release: string;
	operation: string;
	message: string;
}
