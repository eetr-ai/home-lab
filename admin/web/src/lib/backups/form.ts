import type { BackupDatabase, BackupFolder, BackupScheduleRequest } from "@/lib/api/types";

/**
 * The schedule form's state, and turning it into a request.
 *
 * The checks here mirror the API's so a mistake is pointed at before a round
 * trip, not instead of one: the API checks again, and its answer is the one that
 * counts. Each message names the field, because the form shows one banner.
 */

export interface ScheduleForm {
	name: string;
	cron: string;
	timeZone: string;
	/** engine/name keys, the way the checkboxes hold them. */
	databases: string[];
	folder: BackupFolder | null;
	keep: string;
	suspended: boolean;
}

export const MAX_NAME = 45;
export const MAX_KEEP = 100;

const NAME = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;

/** The key a checkbox holds for one database. */
export function databaseKey(database: BackupDatabase): string {
	return `${database.engine}/${database.name}`;
}

export function emptyForm(timeZone: string): ScheduleForm {
	return {
		name: "",
		cron: "0 3 * * *",
		timeZone,
		databases: [],
		folder: null,
		keep: "7",
		suspended: false,
	};
}

/** A form holding an existing schedule, for editing. */
export function formFrom(schedule: BackupScheduleRequest & { name: string }): ScheduleForm {
	return {
		name: schedule.name,
		cron: schedule.cron,
		timeZone: schedule.timeZone ?? "",
		databases: schedule.databases.map(databaseKey),
		folder: schedule.folder.id ? schedule.folder : null,
		keep: String(schedule.keep),
		suspended: schedule.suspended,
	};
}

/** The request the form describes, or the first thing wrong with it. */
export function toRequest(
	form: ScheduleForm,
): { ok: true; request: BackupScheduleRequest } | { ok: false; error: string } {
	const name = form.name.trim();
	if (name.length > MAX_NAME || !NAME.test(name)) {
		return {
			ok: false,
			error: `The name must be lowercase letters, digits and dashes, at most ${MAX_NAME} characters.`,
		};
	}
	if (form.cron.trim() === "") return { ok: false, error: "Choose when the backup runs." };
	if (form.databases.length === 0) return { ok: false, error: "Choose at least one database." };
	if (!form.folder) return { ok: false, error: "Choose the Drive folder to back up into." };

	const keep = Number(form.keep);
	if (!Number.isInteger(keep) || keep < 1 || keep > MAX_KEEP) {
		return { ok: false, error: `Keep between 1 and ${MAX_KEEP} backups of each database.` };
	}

	return {
		ok: true,
		request: {
			name,
			cron: form.cron.trim(),
			timeZone: form.timeZone.trim() || undefined,
			databases: form.databases.map(parseKey),
			folder: form.folder,
			keep,
			suspended: form.suspended,
		},
	};
}

function parseKey(key: string): BackupDatabase {
	const slash = key.indexOf("/");
	return { engine: key.slice(0, slash) as BackupDatabase["engine"], name: key.slice(slash + 1) };
}
