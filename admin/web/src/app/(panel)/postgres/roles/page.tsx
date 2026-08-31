import { listRoles } from "@/app/actions/postgres";
import { listNamespaces } from "@/app/actions/kube";
import { RoleList } from "./_components/role-list";

export const dynamic = "force-dynamic";

/**
 * The roles, and the namespaces a new one's credential can be installed into.
 *
 * Protected namespaces are filtered out here rather than in the form, the same
 * way the Helm deployments page does it: the API refuses them, and offering a
 * choice that would be refused is worse than not offering it. The two failures
 * are reported separately, because "no namespaces" and "the namespaces could not
 * be read" are different sentences.
 */
export default async function PostgresRolesPage() {
	const [roles, namespaces] = await Promise.all([listRoles(), listNamespaces()]);
	return (
		<RoleList
			roles={roles.ok ? roles.data : []}
			loadError={roles.ok ? null : roles.error}
			namespaces={namespaces.ok ? namespaces.data.filter((one) => !one.protected) : []}
			namespacesError={namespaces.ok ? null : namespaces.error}
		/>
	);
}
