import { listRoles } from "@/app/actions/postgres";
import { RoleList } from "./_components/role-list";

export const dynamic = "force-dynamic";

export default async function PostgresRolesPage() {
	const roles = await listRoles();
	return (
		<RoleList roles={roles.ok ? roles.data : []} loadError={roles.ok ? null : roles.error} />
	);
}
