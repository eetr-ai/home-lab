import { call, seg } from "./http";
import type { ActionResult } from "./result";
import type {
	BackupFile,
	BackupRun,
	BackupSchedule,
	BackupScheduleRequest,
	ConnectGoogle,
	GoogleAccessToken,
	GoogleStatus,
} from "./backups-types";

/**
 * The backup operations. Mirrors admin/api/internal/backup.
 *
 * Two halves: the Google connection, and the schedules, which are CronJobs. The
 * grant itself never comes back through here — the API keeps it in a Secret the
 * backup pods read, and the most the browser ever holds is the hour-long Picker
 * token.
 */

export function googleStatus(): Promise<ActionResult<GoogleStatus>> {
	return call<GoogleStatus>("GET", "/api/backups/google");
}

export function connectGoogle(request: ConnectGoogle): Promise<ActionResult<GoogleStatus>> {
	return call<GoogleStatus>("POST", "/api/backups/google/connect", request);
}

export function disconnectGoogle(): Promise<ActionResult<void>> {
	return call<void>("DELETE", "/api/backups/google");
}

export function pickerToken(): Promise<ActionResult<GoogleAccessToken>> {
	return call<GoogleAccessToken>("POST", "/api/backups/google/access-token");
}

export function listSchedules(): Promise<ActionResult<BackupSchedule[]>> {
	return call<BackupSchedule[]>("GET", "/api/backups/schedules");
}

export function readSchedule(name: string): Promise<ActionResult<BackupSchedule>> {
	return call<BackupSchedule>("GET", `/api/backups/schedules/${seg(name)}`);
}

export function createSchedule(request: BackupScheduleRequest): Promise<ActionResult<BackupSchedule>> {
	return call<BackupSchedule>("POST", "/api/backups/schedules", request);
}

export function updateSchedule(
	name: string,
	request: BackupScheduleRequest,
): Promise<ActionResult<BackupSchedule>> {
	return call<BackupSchedule>("PUT", `/api/backups/schedules/${seg(name)}`, request);
}

export function deleteSchedule(name: string): Promise<ActionResult<void>> {
	return call<void>("DELETE", `/api/backups/schedules/${seg(name)}`);
}

export function runSchedule(name: string): Promise<ActionResult<BackupRun>> {
	return call<BackupRun>("POST", `/api/backups/schedules/${seg(name)}/run`);
}

export function listRuns(name: string): Promise<ActionResult<BackupRun[]>> {
	return call<BackupRun[]>("GET", `/api/backups/schedules/${seg(name)}/runs`);
}

export function listFiles(name: string): Promise<ActionResult<BackupFile[]>> {
	return call<BackupFile[]>("GET", `/api/backups/schedules/${seg(name)}/files`);
}
