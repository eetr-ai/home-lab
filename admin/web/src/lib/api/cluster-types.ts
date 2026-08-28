/**
 * The cluster wire types, mirroring admin/api/internal/kube/types.go.
 *
 * Split out of types.ts rather than living beside the database ones: the cluster
 * surface is now the larger half, and one file holding both was past the length
 * the lint allows. Import either from `@/lib/api/types`, which re-exports these.
 */

export interface Namespace {
	name: string;
	status: string;
	/** RFC 3339. */
	createdAt: string;
	labels?: Record<string, string>;
	/**
	 * Whether the panel may delete it. Decided by the API from the name and the
	 * labels, not by the cluster, so the panel renders the answer rather than
	 * working one out of its own.
	 */
	protected: boolean;
	/** Why, when it is. Shown next to the namespace in place of a delete action. */
	protectedReason?: string;
}

/** POST /api/kubernetes/namespaces */
export interface CreateNamespace {
	name: string;
	/**
	 * The API applies its own labels over these — Pod Security enforcement, who
	 * manages it, and the Helm marker — and refuses any key under kubernetes.io
	 * or k8s.io.
	 */
	labels?: Record<string, string>;
}

export interface Workload {
	name: string;
	namespace: string;
	/** Deployment, StatefulSet, DaemonSet. */
	kind: string;
	ready: number;
	desired: number;
	images: string[];
	createdAt: string;
}

export interface Pod {
	name: string;
	namespace: string;
	node: string;
	phase: string;
	/** A summarized state — "Running", "CrashLoopBackOff", "Init:1/2", … */
	status: string;
	/** Containers ready over containers total, e.g. "1/2". */
	ready: string;
	restarts: number;
	/** Every container by name; a log request must name one when there is more than one. Init containers last. */
	containers: string[];
	createdAt: string;
}

export interface ClusterEvent {
	namespace: string;
	/** The involved object, as "Kind/name". */
	object: string;
	type: string;
	reason: string;
	message: string;
	count: number;
	lastSeen: string;
}

/**
 * A quantity of compute, in the units an operator reads: CPU in millicores
 * because that is how requests are written, memory in bytes because that is what
 * a size formatter wants.
 */
export interface Resources {
	cpuMillis: number;
	memoryBytes: number;
	pods?: number;
	ephemeralBytes?: number;
}

/** A node's root disk, as its kubelet reports it. */
export interface Filesystem {
	capacityBytes: number;
	usedBytes: number;
	availableBytes: number;
}

export interface ClusterNode {
	name: string;
	/** "Ready", "NotReady", or "Ready,SchedulingDisabled" for a cordoned node. */
	status: string;
	ready: boolean;
	roles: string[];
	version: string;
	os: string;
	/** The pressure conditions that are true right now. Empty is healthy. */
	pressure: string[];
	capacity: Resources;
	allocatable: Resources;
	/** What the pods scheduled here have reserved, used or not. */
	requested: Resources;
	/**
	 * What is actually being consumed. Absent when metrics-server is not
	 * installed or has not collected a sample yet — which is a normal state, and
	 * why this is optional rather than zero.
	 */
	usage?: Resources;
	/** Absent unless the panel is configured to read node stats from the kubelet. */
	filesystem?: Filesystem;
	createdAt: string;
}

export interface VolumeClaim {
	name: string;
	namespace: string;
	/** Bound, Pending, or Lost. */
	status: string;
	requestedBytes: number;
	/** What was actually provisioned. Zero while the claim is pending. */
	capacityBytes: number;
	storageClass: string;
	volumeName: string;
	accessModes: string[];
	createdAt: string;
}

export interface Volume {
	name: string;
	status: string;
	capacityBytes: number;
	storageClass: string;
	/** "namespace/name" of the bound claim, empty when unbound. */
	claim: string;
	reclaimPolicy: string;
	accessModes: string[];
	createdAt: string;
}

export interface Storage {
	claims: VolumeClaim[];
	volumes: Volume[];
}

export interface ClusterSummary {
	nodes: NodeSummary;
	pods: PodSummary;
	workloads: WorkloadSummary;
	storage: StorageSummary;
	namespaces: number;
	/**
	 * Whether the usage figures were measured at all. When false they are zero
	 * because nothing measured them, not because nothing is running.
	 */
	metricsAvailable: boolean;
}

export interface NodeSummary {
	total: number;
	ready: number;
	pressure: number;
	allocatable: Resources;
	requested: Resources;
	usage: Resources;
}

export interface PodSummary {
	total: number;
	running: number;
	pending: number;
	failed: number;
	restarts: number;
}

export interface WorkloadSummary {
	total: number;
	/** Fewer ready replicas than desired. A workload scaled to zero is not one. */
	degraded: number;
}

export interface StorageSummary {
	claims: number;
	/**
	 * Every claim that is not Bound — Pending and Lost together. They mean the
	 * same thing to an operator: a workload asked for storage and does not have
	 * it. Calling a Lost claim "pending" would suggest it is still coming.
	 */
	unbound: number;
	/** Bound claims only: an unbound claim has no capacity to count. */
	capacityBytes: number;
}

/** A workload's replica count: what was asked for, and what there is. */
export interface Scale {
	replicas: number;
	/** How many exist right now, which lags `replicas` during a rollout. */
	current: number;
}

/** One Kubernetes Service — what gives a workload an address. */
export interface ClusterService {
	name: string;
	namespace: string;
	type: string;
	clusterIP: string;
	/** Rendered as "80→8080/TCP". */
	ports: string[];
	selector: string[];
}

/** One controller condition. */
export interface Condition {
	type: string;
	status: string;
	reason: string;
	message: string;
	/** RFC 3339. When it last changed — whether a fault is new or long-standing. */
	lastTransition: string;
}

export interface WorkloadDetail {
	workload: Workload;
	/** Absent for a DaemonSet, which has no replica count to set. */
	scale?: Scale;
	updated: number;
	available: number;
	conditions: Condition[];
	pods: Pod[];
	services: ClusterService[];
	claims: VolumeClaim[];
	events: ClusterEvent[];
}
