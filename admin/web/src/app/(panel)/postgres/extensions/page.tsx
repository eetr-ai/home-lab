import { listDatabases, listExtensions } from "@/app/actions/postgres";
import { ExtensionList } from "./_components/extension-list";

export const dynamic = "force-dynamic";

/**
 * Extensions are per-database, so this page has no meaning without one. The
 * chosen database is in the query string rather than in component state: it is
 * part of what the page is showing, so it should be linkable and survive a
 * reload.
 */
export default async function PostgresExtensionsPage({
	searchParams,
}: {
	searchParams: Promise<{ database?: string }>;
}) {
	const [{ database: requested }, databases] = await Promise.all([
		searchParams,
		listDatabases(),
	]);

	const names = databases.ok ? databases.data.map((entry) => entry.name) : [];
	// A database named in the URL that no longer exists falls back to the first
	// rather than showing an error: the link is stale, not wrong.
	const selected = requested && names.includes(requested) ? requested : (names[0] ?? "");

	const extensions = selected ? await listExtensions(selected) : null;

	return (
		<ExtensionList
			databases={names}
			selected={selected}
			extensions={extensions?.ok ? extensions.data : []}
			loadError={
				(!databases.ok && databases.error) ||
				(extensions && !extensions.ok && extensions.error) ||
				null
			}
		/>
	);
}
