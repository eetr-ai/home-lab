import { listDatabases, listUsers } from "@/app/actions/mongo";
import { MongoUserList } from "./_components/mongo-user-list";

export const dynamic = "force-dynamic";

/**
 * MongoDB users belong to the database they authenticate against, so this page —
 * like collections — is scoped to one.
 */
export default async function MongoUsersPage({
	searchParams,
}: {
	searchParams: Promise<{ database?: string }>;
}) {
	const [{ database: requested }, databases] = await Promise.all([searchParams, listDatabases()]);

	const names = databases.ok ? databases.data.map((entry) => entry.name) : [];
	const selected = requested && names.includes(requested) ? requested : (names[0] ?? "");
	const users = selected ? await listUsers(selected) : null;

	return (
		<MongoUserList
			databases={names}
			selected={selected}
			users={users?.ok ? users.data : []}
			loadError={
				(!databases.ok && databases.error) || (users && !users.ok && users.error) || null
			}
		/>
	);
}
