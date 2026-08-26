import { listCollections, listDatabases } from "@/app/actions/mongo";
import { Banner } from "@/components/ui/banner";
import { FindConsole } from "./_components/find-console";

export const dynamic = "force-dynamic";

/**
 * The read-only document browser, scoped to one database the way the other
 * MongoDB tabs are — a collection belongs to a database, so the picker is not
 * optional here.
 */
export default async function MongoQueryPage({
	searchParams,
}: {
	searchParams: Promise<{ database?: string }>;
}) {
	const [{ database: requested }, databases] = await Promise.all([searchParams, listDatabases()]);

	const names = databases.ok ? databases.data.map((entry) => entry.name) : [];
	const selected = requested && names.includes(requested) ? requested : (names[0] ?? "");
	const collections = selected ? await listCollections(selected) : null;

	const error =
		(!databases.ok && databases.error) ||
		(collections && !collections.ok && collections.error) ||
		null;

	return (
		<>
			<Banner variant="error" message={error} />
			{/* Keyed by database. The picker changes it with a router push, which
			    re-renders this page but does not remount the console — so without
			    the key the chosen collection would survive into a database that
			    does not have it, and the find would quietly return nothing. */}
			<FindConsole
				key={selected}
				databases={names}
				database={selected}
				collections={collections?.ok ? collections.data.map((entry) => entry.name) : []}
			/>
		</>
	);
}
