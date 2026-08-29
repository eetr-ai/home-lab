"use server";

import { revalidatePath } from "next/cache";

import * as helm from "@/lib/api/helm";
import { withRead, withWrite } from "./_auth";
import type { ActionResult } from "@/lib/api/result";
import type {
	AddDeploymentVersion,
	DeclareDeployment,
	HelmAccepted,
	HelmChartVersion,
	HelmDeployment,
	HelmDeploymentDetail,
	HelmDeploymentSummary,
	HelmDeploymentVersion,
	HelmJob,
	HelmRelease,
	HelmReleaseDetail,
	HelmRevision,
	RolloutDeployment,
} from "@/lib/api/types";

/**
 * The Helm section's actions.
 *
 * One gate applies to the writes here, and it is this one: withWrite checks
 * ADMIN_WRITE_EMAILS, which decides whether *this operator* may change anything.
 * The API behind it authenticates a caller and does not authorize one — any token
 * it accepts may call any route — so nothing else is standing here until the
 * platform's authorization module arrives.
 *
 * Rollouts return an acceptance rather than a result: Helm waits for pods, so
 * what happened is read back from the deployment afterwards. Revalidating the
 * section refreshes the page, which will show the release as pending until it is
 * not.
 */

export async function listDeployments(
	namespace?: string,
): Promise<ActionResult<HelmDeploymentSummary[]>> {
	return withRead(() => helm.listDeployments(namespace));
}

export async function readDeployment(id: string): Promise<ActionResult<HelmDeploymentDetail>> {
	return withRead(() => helm.readDeployment(id));
}

export async function listChartVersions(
	reference: string,
): Promise<ActionResult<HelmChartVersion[]>> {
	return withRead(() => helm.listChartVersions(reference));
}

export async function declareDeployment(
	request: DeclareDeployment,
): Promise<ActionResult<HelmDeployment>> {
	const result = await withWrite(() => helm.declareDeployment(request));
	if (result.ok) revalidatePath("/helm", "layout");
	return result;
}

export async function forgetDeployment(id: string): Promise<ActionResult<void>> {
	const result = await withWrite(() => helm.forgetDeployment(id));
	if (result.ok) revalidatePath("/helm", "layout");
	return result.ok ? { ok: true, data: undefined } : result;
}

export async function addDeploymentVersion(
	id: string,
	request: AddDeploymentVersion,
): Promise<ActionResult<HelmDeploymentVersion>> {
	const result = await withWrite(() => helm.addDeploymentVersion(id, request));
	if (result.ok) revalidatePath("/helm", "layout");
	return result;
}

export async function rolloutDeployment(
	id: string,
	request: RolloutDeployment,
): Promise<ActionResult<HelmAccepted>> {
	const result = await withWrite(() => helm.rolloutDeployment(id, request));
	if (result.ok) revalidatePath("/helm", "layout");
	return result;
}

export async function listReleases(): Promise<ActionResult<HelmRelease[]>> {
	return withRead(() => helm.listReleases());
}

export async function listNamespaceReleases(
	namespace: string,
): Promise<ActionResult<HelmRelease[]>> {
	return withRead(() => helm.listNamespaceReleases(namespace));
}

export async function readRelease(
	namespace: string,
	release: string,
): Promise<ActionResult<HelmReleaseDetail>> {
	return withRead(() => helm.readRelease(namespace, release));
}

export async function readHistory(
	namespace: string,
	release: string,
): Promise<ActionResult<HelmRevision[]>> {
	return withRead(() => helm.readHistory(namespace, release));
}

export async function rollbackRelease(
	namespace: string,
	release: string,
	revision: number,
): Promise<ActionResult<HelmAccepted>> {
	const result = await withWrite(() => helm.rollbackRelease(namespace, release, revision));
	if (result.ok) revalidatePath("/helm", "layout");
	return result;
}

export async function uninstallRelease(
	namespace: string,
	release: string,
): Promise<ActionResult<void>> {
	const result = await withWrite(() => helm.uninstallRelease(namespace, release));
	if (result.ok) revalidatePath("/helm", "layout");
	// The row-delete helper works in ActionResult<void>, and there is nothing in
	// the acceptance a confirmation button would show.
	return result.ok ? { ok: true, data: undefined } : result;
}

/**
 * The Helm jobs matching a filter, newest first.
 *
 * A read rather than a write: looking at what is running changes nothing, and an
 * operator without write access still needs to see whether a deploy somebody else
 * started has finished.
 */
export async function listHelmJobs(filter: {
	namespace?: string;
	release?: string;
	deployment?: string;
}): Promise<ActionResult<HelmJob[]>> {
	return withRead(() => helm.listJobs(filter));
}

export async function readHelmJob(name: string): Promise<ActionResult<HelmJob>> {
	return withRead(() => helm.readJob(name));
}
