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
	RotateSecret,
	SecretRef,
	SecretSummary,
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

/** The Secrets in a namespace, without any of their contents. */
export async function listSecrets(namespace: string): Promise<ActionResult<SecretSummary[]>> {
	return withRead(() => kube.listSecrets(namespace));
}

/**
 * Removes a Secret.
 *
 * A write, so it is behind the allowlist. What it may reach is decided by the
 * API — a protected namespace, Helm's release storage and ServiceAccount tokens
 * are refused there — and the list already carries that answer per row, so the
 * panel does not draw a button this would come back 403 from.
 */
export async function deleteSecret(namespace: string, name: string): Promise<ActionResult<void>> {
	const result = await withWrite(() => kube.deleteSecret(namespace, name));
	if (result.ok) revalidatePath("/kubernetes", "layout");
	return result;
}

/**
 * Replaces the values of keys a Secret already has.
 *
 * Revalidates the section because the key list is what a row shows and a rotation
 * can change nothing else about it — the values were never on the page.
 */
export async function rotateSecret(
	namespace: string,
	name: string,
	request: RotateSecret,
): Promise<ActionResult<SecretRef>> {
	const result = await withWrite(() => kube.rotateSecret(namespace, name, request));
	if (result.ok) revalidatePath("/kubernetes", "layout");
	return result;
}

/**
 * Enrols a namespace as a Helm target, or repairs one that is set up wrongly.
 *
 * Revalidates the Helm section as well as the cluster one: enrolling a namespace
 * changes what can be deployed into, which is what the deployments page offers.
 */
export async function enrolNamespace(namespace: string): Promise<ActionResult<Namespace>> {
	const result = await withWrite(() => kube.enrolNamespace(namespace));
	if (result.ok) {
		revalidatePath("/kubernetes", "layout");
		revalidatePath("/helm", "layout");
	}
	return result;
}

export async function revokeNamespace(namespace: string): Promise<ActionResult<void>> {
	const result = await withWrite(() => kube.revokeNamespace(namespace));
	if (result.ok) {
		revalidatePath("/kubernetes", "layout");
		revalidatePath("/helm", "layout");
	}
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
