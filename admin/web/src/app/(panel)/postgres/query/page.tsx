import { listDatabases, listTables } from "@/app/actions/postgres";
import { Banner } from "@/components/ui/banner";
import { QueryConsole } from "./_components/query-console";

export const dynamic = "force-dynamic";

/**
 * The read-only SQL console.
 *
 * The databases are fetched here so the console can offer them, and the selected
 * database's tables so it can show a schema tree; everything else is a browser
 * concern.
 *
 * Which database is selected comes from the query string, as it does on every
 * other page here that is scoped to a database — so the console can be linked to,
 * survives a reload, and steps through the back button. Changing the selection
 * re-runs this page, which is what re-fetches the tree for the new database.
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

	// The tree is scoped to the selected database. A failure to read it is the
	// tree's problem alone — the console still runs statements — so it is passed
	// down as the tree's own error rather than replacing the whole page.
	const tables = selected ? await listTables(selected) : null;

	return (
		<QueryConsole
			databases={names}
			selected={selected}
			relations={tables?.ok ? tables.data : []}
			relationsError={tables && !tables.ok ? tables.error : null}
		/>
	);
}
