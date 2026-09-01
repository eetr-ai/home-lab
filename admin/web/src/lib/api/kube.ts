import { call, seg } from "./http";
import type { ActionResult } from "./result";
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

/**
 * Writes an Opaque Secret into a namespace, so a credential the panel just
 * issued reaches the chart that will use it.
 *
 * The API refuses a protected namespace and one the panel does not manage, and
 * answers 409 when a Secret of that name is already there unless `overwrite` is
 * set. Nothing reads a value back: the response carries the keys.
 */
export function putSecret(
	namespace: string,
	name: string,
	request: PutSecret,
): Promise<ActionResult<SecretRef>> {
	return call<SecretRef>(
		"PUT",
		`/api/kubernetes/namespaces/${seg(namespace)}/secrets/${seg(name)}`,
		request,
	);
}

/**
 * The Secrets in a namespace, without any of their contents.
 *
 * Names, types, keys and ages. Each row carries whether the panel will delete or
 * rotate it, so the list and the buttons on it cannot disagree with what the API
 * would answer.
 */
export function listSecrets(namespace: string): Promise<ActionResult<SecretSummary[]>> {
	return call<SecretSummary[]>("GET", `/api/kubernetes/namespaces/${seg(namespace)}/secrets`);
}

/**
 * Remove a Secret.
 *
 * Nothing checks whether a workload is using it. Deleting a Secret a running
 * release reads will break it at the next restart, and that is the operator's
 * call — which is why the confirmation is in front of this rather than inside it.
 */
export function deleteSecret(namespace: string, name: string): Promise<ActionResult<void>> {
	return call<void>("DELETE", `/api/kubernetes/namespaces/${seg(namespace)}/secrets/${seg(name)}`);
}

/**
 * Replace the values of keys a Secret already has.
 *
 * This writes the Secret and stops. Pods already running hold the old value until
 * something restarts them, and the panel says so rather than reporting a rotation
 * as though it had taken effect.
 */
export function rotateSecret(
	namespace: string,
	name: string,
	request: RotateSecret,
): Promise<ActionResult<SecretRef>> {
	return call<SecretRef>(
		"POST",
		`/api/kubernetes/namespaces/${seg(namespace)}/secrets/${seg(name)}/rotate`,
		request,
	);
}

/**
 * Enrols a namespace as a Helm target, or repairs an enrolment that is there and
 * wrong. One call for both, because they are the same request.
 */
export function enrolNamespace(namespace: string): Promise<ActionResult<Namespace>> {
	return call<Namespace>(
		"POST",
		`/api/kubernetes/namespaces/${seg(namespace)}/helm-enrolment`,
	);
}

/** Removes the role bindings, after which the panel can neither deploy into the
 * namespace nor read its releases. */
export function revokeNamespace(namespace: string): Promise<ActionResult<void>> {
	return call<void>(
		"DELETE",
		`/api/kubernetes/namespaces/${seg(namespace)}/helm-enrolment`,
	);
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
