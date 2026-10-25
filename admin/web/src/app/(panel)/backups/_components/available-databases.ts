import type { ActionResult } from "@/lib/api/result";
import type { BackupDatabase } from "@/lib/api/types";

/**
 * Every database a schedule could name, from both servers.
 *
 * A failed listing contributes nothing rather than an error: on a lab that does
 * not manage one of the engines, that listing always fails, and the form should
 * simply not offer it.
 */
export function availableDatabases(
	postgres: ActionResult<{ name: string }[]>,
	mongo: ActionResult<{ name: string }[]>,
): BackupDatabase[] {
	const ret: BackupDatabase[] = [];
	if (postgres.ok) for (const one of postgres.data) ret.push({ engine: "postgres", name: one.name });
	if (mongo.ok) {
		// MongoDB's own databases are the server's bookkeeping, not the lab's data.
		for (const one of mongo.data) {
			if (!["admin", "config", "local"].includes(one.name)) ret.push({ engine: "mongo", name: one.name });
		}
	}
	return ret;
}
