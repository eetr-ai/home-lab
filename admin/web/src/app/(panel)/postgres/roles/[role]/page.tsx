import { notFound } from "next/navigation";
import { listDatabases, listRolePrivileges, listRoles } from "@/app/actions/postgres";
import { BackLink } from "../../../_components/back-link";
import { RolePrivileges } from "./_components/role-privileges";

export const dynamic = "force-dynamic";

/**
 * PostgreSQL's own databases, which are not offered here. template0 accepts no
 * connections at all, so its catalog cannot be read; template1 is copied into
 * every database created after it, so a grant there would quietly spread; and
 * `postgres` is the panel's own administrative database. The API refuses changes
 * in all three, and a database that can only refuse is not worth choosing.
 */
const NOT_OFFERED = ["postgres", "template0", "template1"];

/**
 * One role's privileges, in one database.
 *
 * Privileges are per database — a table's ACL lives in that database's catalog —
 * so the page has no meaning without one, and the choice is in the query string
 * like the extensions tab's: linkable, and it survives a reload. A database named
 * in the URL that no longer exists falls back to the first rather than erroring;
 * the link is stale, not wrong.
 *
 * A role that does not exist is a 404, but only when the listing said so. A
 * listing that failed is shown as a failure, not as "no such role".
 */
export default async function PostgresRolePrivilegesPage({
	params,
	searchParams,
}: {
	params: Promise<{ role: string }>;
	searchParams: Promise<{ database?: string }>;
}) {
	const [{ role: raw }, { database: requested }, databases, roles] = await Promise.all([
		params,
		searchParams,
		listDatabases(),
		listRoles(),
	]);
	const role = decodeURIComponent(raw);
	if (roles.ok && !roles.data.some((one) => one.name === role)) notFound();

	const offered = databases.ok ? databases.data.filter((one) => !NOT_OFFERED.includes(one.name)) : [];
	const names = offered.map((one) => one.name);
	const selected = requested && names.includes(requested) ? requested : (names[0] ?? "");
	const privileges = selected ? await listRolePrivileges(selected, role) : null;

	return (
		<div className="flex flex-col gap-4">
			<BackLink href="/postgres/roles" label="All roles" />
			<RolePrivileges
				// Keyed by database, so choosing another one drops the last one's
				// result banner rather than showing it over the wrong grid.
				key={selected}
				role={role}
				databases={names}
				selected={selected}
				databaseOwner={offered.find((one) => one.name === selected)?.owner ?? ""}
				roles={roles.ok ? roles.data.map((one) => one.name) : []}
				privileges={privileges?.ok ? privileges.data : null}
				loadError={
					(!databases.ok && databases.error) ||
					(!roles.ok && roles.error) ||
					(privileges && !privileges.ok && privileges.error) ||
					null
				}
			/>
		</div>
	);
}
