import { call, seg } from "./http";
import type { ActionResult } from "./result";
import type { ClusterEvent, Namespace, Pod, Workload } from "./types";

/**
 * The cluster operations. Mirrors admin/api/internal/kube.
 *
 * Read-only throughout, and not by omission: the API's ServiceAccount holds a
 * ClusterRole with `get`, `list` and `watch` and nothing else, so there is no
 * write operation here to expose.
 */

export function listNamespaces(): Promise<ActionResult<Namespace[]>> {
	return call<Namespace[]>("GET", "/api/kubernetes/namespaces");
}

export function listWorkloads(namespace: string): Promise<ActionResult<Workload[]>> {
	return call<Workload[]>("GET", `/api/kubernetes/namespaces/${seg(namespace)}/workloads`);
}

export function listPods(namespace: string): Promise<ActionResult<Pod[]>> {
	return call<Pod[]>("GET", `/api/kubernetes/namespaces/${seg(namespace)}/pods`);
}

export function listEvents(namespace: string): Promise<ActionResult<ClusterEvent[]>> {
	return call<ClusterEvent[]>("GET", `/api/kubernetes/namespaces/${seg(namespace)}/events`);
}
