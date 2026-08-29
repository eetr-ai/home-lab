import { call, seg } from "./http";
import type { ActionResult } from "./result";
import type {
	HelmAccepted,
	HelmChartListing,
	HelmRelease,
	HelmReleaseDetail,
	HelmRevision,
	InstallRelease,
	UpgradeRelease,
} from "./helm-types";

/**
 * The Helm operations. Mirrors admin/api/internal/helm.
 *
 * Every mutation here answers 202 rather than a result: Helm waits for pods, and
 * that outlasts this client's twenty-second timeout several times over. What
 * happened is read back from the release itself, which is why there is no
 * "install and tell me if it worked" function to reach for.
 */

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

export function listCharts(): Promise<ActionResult<HelmChartListing[]>> {
	return call<HelmChartListing[]>("GET", "/api/helm/charts");
}

export function installRelease(
	namespace: string,
	request: InstallRelease,
): Promise<ActionResult<HelmAccepted>> {
	return call<HelmAccepted>("POST", `/api/helm/namespaces/${seg(namespace)}/releases`, request);
}

export function upgradeRelease(
	namespace: string,
	release: string,
	request: UpgradeRelease,
): Promise<ActionResult<HelmAccepted>> {
	return call<HelmAccepted>(
		"PUT",
		`/api/helm/namespaces/${seg(namespace)}/releases/${seg(release)}`,
		request,
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
