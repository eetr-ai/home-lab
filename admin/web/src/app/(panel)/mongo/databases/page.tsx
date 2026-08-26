import { listDatabases } from "@/app/actions/mongo";
import { MongoDatabaseList } from "./_components/mongo-database-list";

export const dynamic = "force-dynamic";

export default async function MongoDatabasesPage() {
	const databases = await listDatabases();
	return (
		<MongoDatabaseList
			databases={databases.ok ? databases.data : []}
			loadError={databases.ok ? null : databases.error}
		/>
	);
}
