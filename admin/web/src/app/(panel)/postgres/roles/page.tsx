import { listRoles } from "@/app/actions/postgres";
import { listNamespaces } from "@/app/actions/kube";
import { RoleList } from "./_components/role-list";

export const dynamic = "force-dynamic";

/**
 * The roles, and the namespaces a new one's credential can be installed into.
 *
 * The namespaces offered are the ones the Secret write would actually accept:
 * unprotected, and managed by the panel. Filtered here rather than in the form
 * because the API refuses the rest, and a destination that would be refused is
 * worse than none -- the role is created before the Secret is written, so
 * choosing one leaves an operator holding a credential that reached nothing.
 *
 * helmManaged has to come from the API. Half of it is a label the browser can
 * see and half is a list in a values file it cannot.
 *
 * The two failures are reported separately, because "no namespaces" and "the
 * namespaces could not be read" are different sentences.
 */
export default async function PostgresRolesPage() {
	const [roles, namespaces] = await Promise.all([listRoles(), listNamespaces()]);
	return (
		<RoleList
			roles={roles.ok ? roles.data : []}
			loadError={roles.ok ? null : roles.error}
			namespaces={namespaces.ok ? namespaces.data.filter((one) => !one.protected && one.helmManaged) : []}
			namespacesError={namespaces.ok ? null : namespaces.error}
		/>
	);
}
