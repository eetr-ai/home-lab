"use client";

import { Trash2 } from "lucide-react";
import { changeRolePrivileges } from "@/app/actions/postgres";
import { IconButton, InlineDeleteConfirm, Table, TBody, THead, Td, Th } from "@/components/ui";
import type { PostgresDefaultPrivileges, PostgresRolePrivileges } from "@/lib/api/types";
import { ActionsHeader } from "../../../../_components/directory";
import { useRowDelete } from "../../../../_components/use-row-delete";

function entryKey(entry: PostgresDefaultPrivileges): string {
	return JSON.stringify([entry.owner, entry.schema ?? "", entry.objectType]);
}

/**
 * The ALTER DEFAULT PRIVILEGES entries naming the role.
 *
 * Listed apart from the grids because they are not privileges on anything that
 * exists: each says that tables (or sequences) one owner creates later will be
 * granted to this role. The owner column is the point — a table created by any
 * other role is not covered, which is the usual reason a new table "lost" its
 * grants. Removing one revokes exactly the privileges it lists; tables already
 * created keep whatever they were given.
 *
 * Adding one is done from "Grant a schema", where it sits beside the grant on the
 * tables that exist now — the two are almost always wanted together.
 */
export function DefaultPrivilegesList({
	privileges,
	onApplied,
	onError,
}: {
	privileges: PostgresRolePrivileges;
	onApplied: (statements: string[]) => void;
	onError: (message: string | null) => void;
}) {
	const rowDelete = useRowDelete(onError);
	const editable = !privileges.isSuperuser;

	async function revoke(entry: PostgresDefaultPrivileges) {
		const result = await changeRolePrivileges(privileges.database, privileges.role, [
			{
				action: "revoke",
				target: entry.objectType === "tables" ? "defaultTables" : "defaultSequences",
				...(entry.schema ? { schema: entry.schema } : {}),
				owner: entry.owner,
				privileges: entry.privileges,
			},
		]);
		if (!result.ok) return result;
		onApplied(result.data.statements);
		return { ok: true as const, data: undefined };
	}

	return (
		<section>
			<h3 className="mb-1 text-sm font-medium">Default privileges</h3>
			<p className="mb-2 text-sm text-muted-foreground">
				Granted automatically on objects created later — but only on those created by the owner shown.
			</p>
			{privileges.defaults.length === 0 ? (
				<p className="text-sm text-muted-foreground">None. Tables created later will not be granted to {privileges.role}.</p>
			) : (
				<Table>
					<THead>
						<Th>Created by</Th>
						<Th>In schema</Th>
						<Th>Objects</Th>
						<Th>Privileges</Th>
						{editable ? <ActionsHeader /> : null}
					</THead>
					<TBody>
						{privileges.defaults.map((entry) => {
							const key = entryKey(entry);
							return (
								<tr key={key}>
									<Td className="font-medium">{entry.owner}</Td>
									<Td className={entry.schema ? "" : "text-muted-foreground"}>{entry.schema ?? "every schema"}</Td>
									<Td>{entry.objectType}</Td>
									<Td className="text-muted-foreground">{entry.privileges.join(", ")}</Td>
									{editable ? (
										<Td className="text-right">
											{rowDelete.confirmingId === key ? (
												<InlineDeleteConfirm
													label="Revoke?"
													confirmLabel="Revoke"
													busy={rowDelete.isDeleting(key)}
													onConfirm={() => rowDelete.confirm(key, () => revoke(entry))}
													onCancel={rowDelete.cancel}
												/>
											) : (
												<IconButton
													variant="danger"
													aria-label={`Revoke the default privileges from ${entry.owner}`}
													onClick={() => rowDelete.ask(key)}
												>
													<Trash2 className="h-4 w-4" />
												</IconButton>
											)}
										</Td>
									) : null}
								</tr>
							);
						})}
					</TBody>
				</Table>
			)}
		</section>
	);
}
