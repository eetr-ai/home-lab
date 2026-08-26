import { listDatabases } from "@/app/actions/postgres";
import { Banner } from "@/components/ui/banner";
import { QueryConsole } from "./_components/query-console";

export const dynamic = "force-dynamic";

/**
 * The read-only SQL console.
 *
 * The databases are fetched here so the console can offer them; everything else
 * is a browser concern.
 */
export default async function PostgresQueryPage() {
	const databases = await listDatabases();

	if (!databases.ok) {
		return <Banner variant="error" message={databases.error} />;
	}

	return <QueryConsole databases={databases.data.map((database) => database.name)} />;
}
