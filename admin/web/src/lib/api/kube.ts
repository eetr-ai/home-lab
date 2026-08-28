import { call, seg } from "./http";
import type { ActionResult } from "./result";
import type {
	ClusterEvent,
	ClusterNode,
	ClusterSummary,
	CreateNamespace,
	Namespace,
	Pod,
	Storage,
	Workload,
	WorkloadDetail,
} from "./types";

/**
 * The cluster operations. Mirrors admin/api/internal/kube.
 *
 * Reads, plus the two writes the panel offers: rolling a workload's pods and
 * changing its replica count. Both are reversible and neither can create or
 * delete anything — what a workload *is* still comes from this repository's Helm
 * releases. The API's ClusterRole holds exactly the verbs for those two and
 * nothing more.
 *
 * Namespaces can also be created and deleted, which is the one place the panel
 * brings something into being. What it may not delete is decided by the API, and
 * a namespace arrives already carrying that answer.
 *
 * Pod logs are not here. They stream, and `call()` buffers a whole response and
 * gives up after twenty seconds — see src/app/api/kubernetes/logs/route.ts.
 */

export function listNamespaces(): Promise<ActionResult<Namespace[]>> {
	return call<Namespace[]>("GET", "/api/kubernetes/namespaces");
}

export function readNamespace(namespace: string): Promise<ActionResult<Namespace>> {
	return call<Namespace>("GET", `/api/kubernetes/namespaces/${seg(namespace)}`);
}

export function createNamespace(request: CreateNamespace): Promise<ActionResult<Namespace>> {
	return call<Namespace>("POST", "/api/kubernetes/namespaces", request);
}

/**
 * Deletes a namespace and everything in it.
 *
 * `force` is only about emptiness — the API refuses a namespace that still runs
 * workloads without it. It is not a way past protection, and a protected
 * namespace is refused either way.
 */
export function deleteNamespace(namespace: string, force: boolean): Promise<ActionResult<void>> {
	const query = force ? "?force=true" : "";
	return call<void>("DELETE", `/api/kubernetes/namespaces/${seg(namespace)}${query}`);
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

export function listNodes(): Promise<ActionResult<ClusterNode[]>> {
	return call<ClusterNode[]>("GET", "/api/kubernetes/nodes");
}

export function readStorage(): Promise<ActionResult<Storage>> {
	return call<Storage>("GET", "/api/kubernetes/storage");
}

export function readSummary(): Promise<ActionResult<ClusterSummary>> {
	return call<ClusterSummary>("GET", "/api/kubernetes/summary");
}

export function readWorkload(
	kind: string,
	namespace: string,
	name: string,
): Promise<ActionResult<WorkloadDetail>> {
	return call<WorkloadDetail>(
		"GET",
		`/api/kubernetes/namespaces/${seg(namespace)}/workloads/${seg(kind)}/${seg(name)}`,
	);
}

export function restartWorkload(
	kind: string,
	namespace: string,
	name: string,
): Promise<ActionResult<void>> {
	return call<void>(
		"POST",
		`/api/kubernetes/namespaces/${seg(namespace)}/workloads/${seg(kind)}/${seg(name)}/restart`,
	);
}

export function scaleWorkload(
	kind: string,
	namespace: string,
	name: string,
	replicas: number,
): Promise<ActionResult<void>> {
	return call<void>(
		"PUT",
		`/api/kubernetes/namespaces/${seg(namespace)}/workloads/${seg(kind)}/${seg(name)}/scale`,
		{ replicas },
	);
}
