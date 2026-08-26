import { listCollections, listDatabases } from "@/app/actions/mongo";
import { CollectionList } from "./_components/collection-list";

export const dynamic = "force-dynamic";

export default async function MongoCollectionsPage({
	searchParams,
}: {
	searchParams: Promise<{ database?: string }>;
}) {
	const [{ database: requested }, databases] = await Promise.all([searchParams, listDatabases()]);

	const names = databases.ok ? databases.data.map((entry) => entry.name) : [];
	const selected = requested && names.includes(requested) ? requested : (names[0] ?? "");
	const collections = selected ? await listCollections(selected) : null;

	return (
		<CollectionList
			databases={names}
			selected={selected}
			collections={collections?.ok ? collections.data : []}
			loadError={
				(!databases.ok && databases.error) ||
				(collections && !collections.ok && collections.error) ||
				null
			}
		/>
	);
}
