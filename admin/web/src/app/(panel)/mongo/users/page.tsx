import { listDatabases, listUsers } from "@/app/actions/mongo";
import { listNamespaces } from "@/app/actions/kube";
import { MongoUserList } from "./_components/mongo-user-list";

export const dynamic = "force-dynamic";

/**
 * MongoDB users belong to the database they authenticate against, so this page —
 * like collections — is scoped to one.
 *
 * The namespaces offered are the ones the Secret write would actually accept:
 * unprotected, and managed by the panel. Filtered here rather than in the form
 * because the API refuses the rest, and a destination that would be refused is
 * worse than none -- the role is created before the Secret is written, so
 * choosing one leaves an operator holding a credential that reached nothing.
 *
 * helmManaged has to come from the API. Half of it is a label the browser can
 * see and half is a list in a values file it cannot.
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
			namespaces={namespaces.ok ? namespaces.data.filter((one) => !one.protected && one.helmManaged) : []}
			namespacesError={namespaces.ok ? null : namespaces.error}
		/>
	);
}
