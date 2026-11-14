import { listNamespaces } from "@/app/actions/kube";
import { listUsers } from "@/app/actions/redis";
import { RedisUserList } from "./_components/redis-user-list";

export const dynamic = "force-dynamic";

/**
 * The ACL users apps authenticate as.
 *
 * The namespaces offered for installing a credential are the ones the Secret
 * write would accept — unprotected and managed by the panel — for the reason the
 * MongoDB users page gives.
 */
export default async function RedisUsersPage() {
	const [users, namespaces] = await Promise.all([listUsers(), listNamespaces()]);

	return (
		<RedisUserList
			users={users.ok ? users.data.users : []}
			// Unknown is not the same as "not persistent". Only a list that was read
			// can say the server has no aclfile; a failed read says nothing either way.
			persistent={users.ok ? users.data.persistent : true}
			loadError={users.ok ? null : users.error}
			namespaces={namespaces.ok ? namespaces.data.filter((one) => !one.protected && one.helmManaged) : []}
			namespacesError={namespaces.ok ? null : namespaces.error}
		/>
	);
}
