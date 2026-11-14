import { listNamespaces } from "@/app/actions/kube";
import { AccessList } from "./_components/access-list";

export const dynamic = "force-dynamic";

/**
 * Which namespaces the Redis NetworkPolicy admits.
 *
 * This is where the traffic may come from, not who may authenticate: a pod in an
 * admitted namespace still needs a user from the Users tab. Both are needed, and
 * they are decided by different things, which is why they are different tabs.
 */
export default async function RedisAccessPage() {
	const namespaces = await listNamespaces();
	return (
		<AccessList
			namespaces={namespaces.ok ? namespaces.data : []}
			loadError={namespaces.ok ? null : namespaces.error}
		/>
	);
}
