/**
 * The Redis slice's wire types. Mirrors admin/api/internal/redis/types.go.
 */

/** What the server says about itself, parsed from INFO. */
export interface RedisOverview {
	version: string;
	mode: string;
	uptimeSeconds: number;
	usedMemoryBytes: number;
	peakMemoryBytes: number;
	/** Zero when the server has no ceiling. */
	maxMemoryBytes: number;
	maxMemoryPolicy: string;
	connectedClients: number;
	blockedClients: number;
	maxClients: number;
	rejectedConnections: number;
	commandsProcessed: number;
	keyspaceHits: number;
	keyspaceMisses: number;
	expiredKeys: number;
	evictedKeys: number;
	/** Commands refused for want of memory since the server started. */
	oomRejections: number;
	keyspace: RedisKeyspace[];
}

export interface RedisKeyspace {
	database: number;
	keys: number;
	expires: number;
}

/**
 * A value out of Redis, which stores bytes. Text that is not UTF-8 arrives
 * base64, and `encoding` says which.
 */
export interface RedisText {
	value: string;
	encoding: "utf8" | "base64";
}

export interface RedisKeySummary {
	key: RedisText;
	type: string;
	/** Milliseconds left, or -1 for a key that never expires. */
	ttlMillis: number;
}

export interface RedisKeyPage {
	keys: RedisKeySummary[];
	/** Empty when the scan is finished, and only then. */
	nextCursor: string;
}

export interface RedisField {
	name: RedisText;
	value: RedisText;
}

export interface RedisScoredMember {
	member: RedisText;
	score: number;
}

export interface RedisStreamEntry {
	id: string;
	fields: RedisField[];
}

/** One key opened up. The value is bounded, and `truncated` says whether there was more. */
export interface RedisKeyDetail {
	key: RedisText;
	type: string;
	ttlMillis: number;
	memoryBytes: number;
	length: number;
	truncated: boolean;
	supported: boolean;
	string?: RedisText;
	items?: RedisText[];
	fields?: RedisField[];
	members?: RedisScoredMember[];
	entries?: RedisStreamEntry[];
}

export type RedisPreset = "readwrite" | "readonly";

export interface RedisUser {
	name: string;
	enabled: boolean;
	hasPassword: boolean;
	prefix?: string;
	preset: RedisPreset | "custom";
	/** Only users whose rules are exactly a prefix and a preset can be edited here. */
	editable: boolean;
}

export interface RedisUserList {
	/** False on a Redis with no aclfile, where users would not survive a restart. */
	persistent: boolean;
	users: RedisUser[];
}

export interface CreateRedisUser {
	name: string;
	password: string;
	prefix: string;
	preset: RedisPreset;
}

export interface UpdateRedisUser {
	prefix: string;
	preset: RedisPreset;
	/** Empty keeps the current password. */
	password?: string;
}
