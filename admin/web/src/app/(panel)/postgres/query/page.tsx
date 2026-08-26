import { listDatabases } from "@/app/actions/postgres";
import { Banner } from "@/components/ui/banner";
import { QueryConsole } from "./_components/query-console";

export const dynamic = "force-dynamic";

/**
 * The read-only SQL console.
 *
 * The databases are fetched here so the console can offer them; everything else
 * is a browser concern.
 *
 * Which one is selected comes from the query string, as it does on every other
 * page here that is scoped to a database. It used to be component state, which
 * made this the one such page that could not be linked to: a reload lost the
 * choice, the back button stepped over it, and the assistant — which can only
 * hand somebody a URL — could open the console but not open it on the database
 * it had just been talking about.
 */
export default async function PostgresQueryPage({
	searchParams,
}: {
	searchParams: Promise<{ database?: string }>;
}) {
	const [{ database: requested }, databases] = await Promise.all([searchParams, listDatabases()]);

	if (!databases.ok) {
		return <Banner variant="error" message={databases.error} />;
	}

	const names = databases.data.map((database) => database.name);
	// A database named in the URL that no longer exists falls back to the first
	// rather than showing an error: the link is stale, not wrong.
	const selected = requested && names.includes(requested) ? requested : (names[0] ?? "");

	return <QueryConsole databases={names} selected={selected} />;
}
