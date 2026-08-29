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
//
// In cluster-types.ts, re-exported here so a caller has one place to import wire
// types from regardless of which half of the API they came from.

export type {
	ClusterEvent,
	ClusterNode,
	ClusterService,
	ClusterSummary,
	Condition,
	Filesystem,
	CreateNamespace,
	Namespace,
	NodeSummary,
	Pod,
	PodSummary,
	Resources,
	Scale,
	Storage,
	StorageSummary,
	Volume,
	VolumeClaim,
	Workload,
	WorkloadDetail,
	WorkloadSummary,
} from "./cluster-types";

// ── Editing and querying ────────────────────────────────────────────────────

/** The desired state of a PostgreSQL role. Omitting the password leaves it. */
export interface UpdatePostgresRole {
	canLogin: boolean;
	canCreateDatabase: boolean;
	canCreateRole: boolean;
	/** -1 for unlimited, which is PostgreSQL's own default. */
	connectionLimit: number;
	password?: string;
}

/** The desired state of a PostgreSQL database. Only the owner can change. */
export interface UpdatePostgresDatabase {
	owner: string;
}

/** The desired state of a MongoDB user. Omitting the password leaves it. */
export interface UpdateMongoUser {
	roles: MongoRole[];
	password?: string;
}

/** A read-only SQL statement. */
export interface QueryRequest {
	sql: string;
}

export interface QueryResult {
	columns: string[];
	/** Values already rendered as text; NULL is the literal string "NULL". */
	rows: string[][];
	/** The result was cut at the row cap — a partial answer, said to be one. */
	truncated: boolean;
	elapsedMs: number;
}

/** A MongoDB find. Each part is a document, so nothing is parsed as syntax. */
export interface FindRequest {
	collection: string;
	filter?: Record<string, unknown>;
	projection?: Record<string, unknown>;
	sort?: Record<string, unknown>;
	limit?: number;
}

export interface FindResult {
	/** Extended-JSON strings, one per document. */
	documents: string[];
	truncated: boolean;
	elapsedMs: number;
}
