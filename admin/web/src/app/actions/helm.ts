"use server";

import { revalidatePath } from "next/cache";

import * as helm from "@/lib/api/helm";
import { withRead, withWrite } from "./_auth";
import type { ActionResult } from "@/lib/api/result";
import type {
	HelmAccepted,
	HelmChartListing,
	HelmRelease,
	HelmReleaseDetail,
	HelmRevision,
	InstallRelease,
	UpgradeRelease,
} from "@/lib/api/types";

/**
 * The Helm section's actions.
 *
 * Two gates apply to the writes here and they are not the same gate. This layer
 * checks ADMIN_WRITE_EMAILS, which is what decides whether *this operator* may
 * change anything. The API separately requires the admin:deploy scope, which is
 * what bounds a pipeline's token — and which the operator's token satisfies today
 * only because it names no scopes at all.
 *
 * Every mutation returns an acceptance rather than a result: Helm waits for pods,
 * so what happened is read back from the release afterwards. Revalidating the
 * section refreshes the list, which will show the release as pending until it is
 * not.
 */

export async function listReleases(): Promise<ActionResult<HelmRelease[]>> {
	return withRead(() => helm.listReleases());
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

export async function listCharts(): Promise<ActionResult<HelmChartListing[]>> {
	return withRead(() => helm.listCharts());
}

export async function installRelease(
	namespace: string,
	request: InstallRelease,
): Promise<ActionResult<HelmAccepted>> {
	const result = await withWrite(() => helm.installRelease(namespace, request));
	if (result.ok) revalidatePath("/helm", "layout");
	return result;
}

export async function upgradeRelease(
	namespace: string,
	release: string,
	request: UpgradeRelease,
): Promise<ActionResult<HelmAccepted>> {
	const result = await withWrite(() => helm.upgradeRelease(namespace, release, request));
	if (result.ok) revalidatePath("/helm", "layout");
	return result;
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
