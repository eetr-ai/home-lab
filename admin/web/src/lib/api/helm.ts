import { call, seg } from "./http";
import type { ActionResult } from "./result";
import type {
	AddDeploymentVersion,
	DeclareDeployment,
	HelmAccepted,
	HelmChartVersion,
	HelmDeployment,
	HelmDeploymentDetail,
	HelmDeploymentSummary,
	HelmDeploymentVersion,
	HelmRelease,
	HelmReleaseDetail,
	HelmRevision,
	RolloutDeployment,
} from "./helm-types";

/**
 * The Helm operations. Mirrors admin/api/internal/helm.
 *
 * Two families, and the difference matters. The `deployment` calls read and write
 * this lab's record of what should be running. The `release` calls read and act
 * on what actually is, straight out of Helm's storage. Nothing here merges the
 * two — the API does that, once, so the panel cannot end up with its own opinion.
 *
 * Every mutation answers 202 rather than a result: Helm waits for pods, and that
 * outlasts this client's twenty-second timeout several times over.
 */

export function listDeployments(namespace?: string): Promise<ActionResult<HelmDeploymentSummary[]>> {
	const query = namespace ? `?namespace=${encodeURIComponent(namespace)}` : "";
	return call<HelmDeploymentSummary[]>("GET", `/api/helm/deployments${query}`);
}

export function readDeployment(id: string): Promise<ActionResult<HelmDeploymentDetail>> {
	return call<HelmDeploymentDetail>("GET", `/api/helm/deployments/${seg(id)}`);
}

export function declareDeployment(
	request: DeclareDeployment,
): Promise<ActionResult<HelmDeployment>> {
	return call<HelmDeployment>("POST", "/api/helm/deployments", request);
}

export function forgetDeployment(id: string): Promise<ActionResult<void>> {
	return call<void>("DELETE", `/api/helm/deployments/${seg(id)}`);
}

export function addDeploymentVersion(
	id: string,
	request: AddDeploymentVersion,
): Promise<ActionResult<HelmDeploymentVersion>> {
	return call<HelmDeploymentVersion>("POST", `/api/helm/deployments/${seg(id)}/versions`, request);
}

export function rolloutDeployment(
	id: string,
	request: RolloutDeployment,
): Promise<ActionResult<HelmAccepted>> {
	return call<HelmAccepted>("POST", `/api/helm/deployments/${seg(id)}/rollout`, request);
}

export function listChartVersions(reference: string): Promise<ActionResult<HelmChartVersion[]>> {
	return call<HelmChartVersion[]>(
		"GET",
		`/api/helm/chart-versions?ref=${encodeURIComponent(reference)}`,
	);
}

export function listReleases(): Promise<ActionResult<HelmRelease[]>> {
	return call<HelmRelease[]>("GET", "/api/helm/releases");
}

export function listNamespaceReleases(namespace: string): Promise<ActionResult<HelmRelease[]>> {
	return call<HelmRelease[]>("GET", `/api/helm/namespaces/${seg(namespace)}/releases`);
}

export function readRelease(
	namespace: string,
	release: string,
): Promise<ActionResult<HelmReleaseDetail>> {
	return call<HelmReleaseDetail>(
		"GET",
		`/api/helm/namespaces/${seg(namespace)}/releases/${seg(release)}`,
	);
}

export function readHistory(
	namespace: string,
	release: string,
): Promise<ActionResult<HelmRevision[]>> {
	return call<HelmRevision[]>(
		"GET",
		`/api/helm/namespaces/${seg(namespace)}/releases/${seg(release)}/history`,
	);
}

export function rollbackRelease(
	namespace: string,
	release: string,
	revision: number,
): Promise<ActionResult<HelmAccepted>> {
	return call<HelmAccepted>(
		"POST",
		`/api/helm/namespaces/${seg(namespace)}/releases/${seg(release)}/rollback`,
		{ revision },
	);
}

export function uninstallRelease(
	namespace: string,
	release: string,
): Promise<ActionResult<HelmAccepted>> {
	return call<HelmAccepted>(
		"DELETE",
		`/api/helm/namespaces/${seg(namespace)}/releases/${seg(release)}`,
	);
}
