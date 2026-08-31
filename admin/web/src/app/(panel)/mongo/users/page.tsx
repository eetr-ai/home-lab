import { listDatabases, listUsers } from "@/app/actions/mongo";
import { listNamespaces } from "@/app/actions/kube";
import { MongoUserList } from "./_components/mongo-user-list";

export const dynamic = "force-dynamic";

/**
 * MongoDB users belong to the database they authenticate against, so this page —
 * like collections — is scoped to one.
 *
 * The namespaces come along because a new user's credential can be installed as
 * a Secret. Protected ones are filtered out here rather than in the form: the API
 * refuses them, and offering a choice that would be refused is worse than not
 * offering it.
 */
export default async function MongoUsersPage({
	searchParams,
}: {
	searchParams: Promise<{ database?: string }>;
}) {
	const [{ database: requested }, databases, namespaces] = await Promise.all([
		searchParams,
		listDatabases(),
		listNamespaces(),
	]);

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
			namespaces={namespaces.ok ? namespaces.data.filter((one) => !one.protected) : []}
			namespacesError={namespaces.ok ? null : namespaces.error}
		/>
	);
}
