import { listDatabases, listRoles } from "@/app/actions/postgres";
import { DatabaseList } from "./_components/database-list";

export const dynamic = "force-dynamic";

/**
 * Fetches on the server and hands the rows to a client component, which owns only
 * what needs a browser: the create panel and the delete confirmations.
 *
 * The roles come along because creating a database offers them as owners. One
 * round trip either way — they are fetched in parallel rather than in sequence.
 */
export default async function PostgresDatabasesPage() {
	const [databases, roles] = await Promise.all([listDatabases(), listRoles()]);

	return (
		<DatabaseList
			databases={databases.ok ? databases.data : []}
			owners={roles.ok ? roles.data.map((role) => role.name) : []}
			loadError={databases.ok ? null : databases.error}
		/>
	);
}
