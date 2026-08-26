"use server";

import { revalidatePath } from "next/cache";

import * as kube from "@/lib/api/kube";
import { withRead, withWrite } from "./_auth";
import type { ActionResult } from "@/lib/api/result";
import type {
	ClusterEvent,
	ClusterNode,
	ClusterSummary,
	Namespace,
	Pod,
	Storage,
	Workload,
	WorkloadDetail,
} from "@/lib/api/types";

/**
 * The cluster section's actions.
 *
 * Mostly reads. The two writes are wrapped in withWrite, which is where the
 * ADMIN_WRITE_EMAILS allowlist is enforced — the API itself does not authorize
 * per endpoint, so this is the only thing standing between a signed-in operator
 * and restarting anything on the cluster. Worth knowing before that list is left
 * unset, which permits everyone.
 */

export async function listNamespaces(): Promise<ActionResult<Namespace[]>> {
	return withRead(() => kube.listNamespaces());
}

export async function listWorkloads(namespace: string): Promise<ActionResult<Workload[]>> {
	return withRead(() => kube.listWorkloads(namespace));
}

export async function listPods(namespace: string): Promise<ActionResult<Pod[]>> {
	return withRead(() => kube.listPods(namespace));
}

export async function listEvents(namespace: string): Promise<ActionResult<ClusterEvent[]>> {
	return withRead(() => kube.listEvents(namespace));
}

export async function listNodes(): Promise<ActionResult<ClusterNode[]>> {
	return withRead(() => kube.listNodes());
}

export async function readStorage(): Promise<ActionResult<Storage>> {
	return withRead(() => kube.readStorage());
}

export async function readSummary(): Promise<ActionResult<ClusterSummary>> {
	return withRead(() => kube.readSummary());
}

export async function readWorkload(
	kind: string,
	namespace: string,
	name: string,
): Promise<ActionResult<WorkloadDetail>> {
	return withRead(() => kube.readWorkload(kind, namespace, name));
}

export async function restartWorkload(
	kind: string,
	namespace: string,
	name: string,
): Promise<ActionResult<void>> {
	const result = await withWrite(() => kube.restartWorkload(kind, namespace, name));
	if (result.ok) revalidatePath("/kubernetes", "layout");
	return result;
}

export async function scaleWorkload(
	kind: string,
	namespace: string,
	name: string,
	replicas: number,
): Promise<ActionResult<void>> {
	const result = await withWrite(() => kube.scaleWorkload(kind, namespace, name, replicas));
	if (result.ok) revalidatePath("/kubernetes", "layout");
	return result;
}
