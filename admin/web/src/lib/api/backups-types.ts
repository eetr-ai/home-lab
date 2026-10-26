/**
 * The backups wire types, mirroring admin/api/internal/backup.
 *
 * Their own module for the same reason helm-types.ts is. Import either from
 * `@/lib/api/types`, which re-exports these.
 */

/** GET /api/backups/google. Never carries the grant itself. */
export interface GoogleStatus {
	/** Whether this lab has an OAuth client at all. */
	configured: boolean;
	connected: boolean;
	account?: string;
	clientId?: string;
	scopes?: string[];
	pickerApiKey?: string;
	pickerAppId?: string;
}

/** A short-lived token for Google's Picker, which runs in the browser. */
export interface GoogleAccessToken {
	accessToken: string;
	/** RFC 3339. */
	expiresAt: string;
}

export interface ConnectGoogle {
	code: string;
	codeVerifier: string;
	redirectUri: string;
}

export type BackupEngine = "postgres" | "mongo";

export interface BackupDatabase {
	engine: BackupEngine;
	name: string;
}

export interface BackupFolder {
	id: string;
	/** For showing. Drive is asked by id, so a renamed folder still receives backups. */
	name: string;
}

export interface BackupScheduleRequest {
	/** Required on create; the path names the schedule on replace. */
	name?: string;
	cron: string;
	/** An IANA zone. Empty is the cluster controller's, usually UTC. */
	timeZone?: string;
	databases: BackupDatabase[];
	folder: BackupFolder;
	/** How many backups of each database to keep. */
	keep: number;
	suspended: boolean;
}

export interface BackupSchedule extends BackupScheduleRequest {
	name: string;
	createdAt: string;
	lastScheduled?: string;
	lastSucceeded?: string;
	running: number;
	actor?: string;
}

export type BackupRunPhase = "pending" | "running" | "succeeded" | "failed";

export interface BackupRun {
	name: string;
	phase: BackupRunPhase;
	reason?: string;
	manual: boolean;
	pod?: string;
	namespace: string;
	/** In the order they run: the dumps, then the upload. */
	containers: string[];
	createdAt: string;
	startedAt?: string;
	finishedAt?: string;
}

export interface BackupFile {
	id: string;
	name: string;
	/** engine/name, e.g. postgres/app, or postgres/@globals for the roles. */
	database: string;
	size: number;
	createdAt: string;
	webViewLink?: string;
}
