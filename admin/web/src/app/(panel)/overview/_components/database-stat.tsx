import type { LucideIcon } from "lucide-react";
import type { ActionResult } from "@/lib/api/result";
import { formatBytes } from "@/lib/format/bytes";
import { plural } from "./plural";
import { Stat } from "./stat";

/** Anything the panel lists that reports a size on disk. */
interface Sized {
	sizeBytes: number;
}

/**
 * One engine's disk total.
 *
 * A failure here is reported in the tile rather than as a page-level banner: a
 * PostgreSQL server that is down must not take the MongoDB figure — or the
 * cluster tiles above — with it. The dashboard is most useful exactly when
 * something is broken.
 */
export function DatabaseStat<T extends Sized>({
	label,
	icon,
	result,
}: {
	label: string;
	icon: LucideIcon;
	result: ActionResult<T[]>;
}) {
	if (!result.ok) {
		return <Stat icon={icon} label={label} value="—" detail={result.error} tone="danger" />;
	}

	const databases = result.data;
	const total = databases.reduce((sum, database) => sum + database.sizeBytes, 0);
	// A size the API could not read arrives as zero, and so does a database that
	// genuinely holds nothing. Only the count is certain, so the total is
	// qualified rather than presented as exact.
	const unreadable = databases.filter((database) => database.sizeBytes === 0).length;

	return (
		<Stat
			icon={icon}
			label={label}
			value={formatBytes(total)}
			detail={
				unreadable > 0
					? `${plural(databases.length, "database")}, ${unreadable} reporting no size`
					: `across ${plural(databases.length, "database")}`
			}
		/>
	);
}
