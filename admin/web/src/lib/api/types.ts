/**
 * The admin API's wire types, mirroring the Go structs the OpenAPI description
 * is generated from (the `types.go` in each slice under admin/api/internal).
 *
 * Hand-written rather than generated, deliberately: the surface is small, and a
 * generator would be a build step and a dependency to keep working in exchange
 * for saving a hundred lines that change when the API does. The coverage tests on
 * the Go side already guarantee the description matches the routes; if these ever
 * drift, `task admin-web:typecheck` will not catch it, but the first call will.
 */

/** GET /api/whoami */
export interface Whoami {
	subject: string;
	email: string;
}

// ── PostgreSQL ──────────────────────────────────────────────────────────────

export interface PostgresDatabase {
	name: string;
	owner: string;
	encoding: string;
	/** Zero when the database exists but its size could not be read. */
	sizeBytes: number;
}

export interface PostgresRole {
	name: string;
	canLogin: boolean;
	canCreateDatabase: boolean;
	canCreateRole: boolean;
	isSuperuser: boolean;
	/** -1 means no limit, which is PostgreSQL's own default. */
	connectionLimit: number;
}

export interface PostgresExtension {
	name: string;
	version: string;
}

export interface CreatePostgresDatabase {
	name: string;
	/** Optional; the connecting superuser owns it when omitted. */
	owner?: string;
}

export interface CreatePostgresRole {
	name: string;
	/**
	 * Never sent to PostgreSQL as typed — the API derives a SCRAM-SHA-256
	 * verifier from it and sends that, so the plaintext never reaches the server.
	 */
	password?: string;
	canLogin?: boolean;
	canCreateDatabase?: boolean;
	canCreateRole?: boolean;
}

// ── MongoDB ─────────────────────────────────────────────────────────────────

export interface MongoDatabase {
	name: string;
	sizeBytes: number;
	/**
	 * MongoDB has no empty databases: one exists only while it holds a
	 * collection. This marks the ones created here that are still waiting for
	 * their first write.
	 */
	empty: boolean;
}

export interface MongoCollection {
	name: string;
	/** "collection" or "view". */
	type: string;
}

export interface MongoRole {
	name: string;
	database: string;
}

export interface MongoUser {
	name: string;
	database: string;
	roles: MongoRole[];
}

export interface CreateMongoDatabase {
	name: string;
	/** Required: a database with no collection would not survive being created. */
	collection: string;
}

export interface CreateMongoCollection {
	name: string;
}

export interface CreateMongoUser {
	name: string;
	password: string;
	roles: MongoRole[];
}

// ── Kubernetes ──────────────────────────────────────────────────────────────

export interface Namespace {
	name: string;
	status: string;
	/** RFC 3339. */
	createdAt: string;
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
