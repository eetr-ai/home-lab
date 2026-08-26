"use server";

import * as kube from "@/lib/api/kube";
import { withRead } from "./_auth";
import type { ActionResult } from "@/lib/api/result";
import type {
	ClusterEvent,
	ClusterNode,
	ClusterSummary,
	Namespace,
	Pod,
	Storage,
	Workload,
} from "@/lib/api/types";

/**
 * The cluster section's actions — reads only, and not because the write ones are
 * still to come. The API's ServiceAccount holds a ClusterRole with `get`, `list`
 * and `watch` and nothing else, so there is no write operation to wrap. Changing
 * workloads is what the repository's Helm releases are for.
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
