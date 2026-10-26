import type { BackupFile, RestoreBackup, RestoreMode } from "@/lib/api/types";

/**
 * The restore form's rules, mirrored from the API so a mistake is pointed at
 * before the round trip. The API checks again; its answer is the one that counts.
 */

const NAME = /^[A-Za-z0-9_][A-Za-z0-9_-]{0,62}$/;

/** engine and database, from a file's tag; null for one that is not restorable. */
export function sourceOf(file: BackupFile): { engine: string; database: string } | null {
	const slash = file.database.indexOf("/");
	if (slash < 0) return null;
	const engine = file.database.slice(0, slash);
	const database = file.database.slice(slash + 1);
	// The roles file is not restored on its own: restoring a database creates the
	// roles it needs, and replaying all of them would reset live passwords.
	if (!database || database === "@globals" || (engine !== "postgres" && engine !== "mongo")) return null;
	return { engine, database };
}

/** The name a restore into a new database takes unless told otherwise. */
export function defaultTarget(database: string, now: Date): string {
	const day = now.toISOString().slice(0, 10).replaceAll("-", "");
	return `${database}_restored_${day}`;
}

/** The request the form describes, or what is wrong with it. */
export function restoreRequest(
	database: string,
	mode: RestoreMode,
	target: string,
	confirm: string,
): { ok: true; request: RestoreBackup } | { ok: false; error: string } {
	if (mode === "replace") {
		if (confirm !== database) return { ok: false, error: `Type ${database} to confirm replacing it.` };
		return { ok: true, request: { mode, confirm } };
	}
	const name = target.trim();
	if (name === database) return { ok: false, error: `A new database needs a name other than ${database}.` };
	if (!NAME.test(name)) {
		return { ok: false, error: "The name must be letters, digits, underscores and dashes, up to 63." };
	}
	return { ok: true, request: { mode, target: name } };
}
