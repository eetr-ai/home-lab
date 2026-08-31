"use server";

import { revalidatePath } from "next/cache";

import * as kube from "@/lib/api/kube";
import { withRead, withWrite } from "./_auth";
import type { ActionResult } from "@/lib/api/result";
import type {
	ClusterEvent,
	ClusterNode,
	ClusterSummary,
	CreateNamespace,
	Namespace,
	Pod,
	PutSecret,
	SecretRef,
	Storage,
	Workload,
	WorkloadDetail,
} from "@/lib/api/types";

/**
 * The cluster section's actions.
 *
 * Mostly reads. The writes are wrapped in withWrite, which is where the
 * ADMIN_WRITE_EMAILS allowlist is enforced — and it is the only thing standing
 * between a signed-in operator and the cluster. The API authenticates a caller
 * and does not authorize one: any token it accepts may call any route, until the
 * platform's authorization module arrives. Worth knowing before that list is
 * left unset, which permits everyone.
 */

export async function listNamespaces(): Promise<ActionResult<Namespace[]>> {
	return withRead(() => kube.listNamespaces());
}

export async function createNamespace(
	request: CreateNamespace,
): Promise<ActionResult<Namespace>> {
	const result = await withWrite(() => kube.createNamespace(request));
	if (result.ok) revalidatePath("/kubernetes", "layout");
	return result;
}

/**
 * Deletes a namespace and everything in it.
 *
 * `force` says only that the caller knows it is not empty. A protected namespace
 * is refused by the API either way, and there is no flag here that changes that.
 */
export async function deleteNamespace(namespace: string, force: boolean): Promise<ActionResult<void>> {
	const result = await withWrite(() => kube.deleteNamespace(namespace, force));
	if (result.ok) revalidatePath("/kubernetes", "layout");
	return result;
}

/**
 * Writes a Secret into a namespace.
 *
 * Called after a database role or user is created, with the password that role
 * was given — which is why it revalidates the cluster section and nothing else:
 * the credential itself is not stored anywhere the panel reads back.
 */
export async function putSecret(
	namespace: string,
	name: string,
	request: PutSecret,
): Promise<ActionResult<SecretRef>> {
	const result = await withWrite(() => kube.putSecret(namespace, name, request));
	if (result.ok) revalidatePath("/kubernetes", "layout");
	return result;
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
