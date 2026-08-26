"use client";

import { useState } from "react";
import { Check, Plus, Trash2, Users } from "lucide-react";
import { dropRole } from "@/app/actions/postgres";
import { Button, IconButton, InlineDeleteConfirm, Td, Th } from "@/components/ui";
import { ActionsHeader, Directory } from "../../../_components/directory";
import { useRowDelete } from "../../../_components/use-row-delete";
import type { PostgresRole } from "@/lib/api/types";
import { CreateRolePanel } from "./create-role-panel";

/** A tick or nothing. A column of "yes"/"no" is harder to scan than one of ticks. */
function Flag({ on, label }: { on: boolean; label: string }) {
	return on ? (
		<Check className="h-4 w-4 text-success-icon" aria-label={label} />
	) : (
		<span className="sr-only">{`not ${label}`}</span>
	);
}

export function RoleList({
	roles,
	loadError,
}: {
	roles: PostgresRole[];
	loadError: string | null;
}) {
	const [error, setError] = useState<string | null>(loadError);
	const [creating, setCreating] = useState(false);
	const rowDelete = useRowDelete(setError);

	const create = (
		<Button icon={Plus} onClick={() => setCreating(true)}>
			New role
		</Button>
	);

	return (
		<>
			<div className="mb-4 flex justify-end">{create}</div>

			<Directory
				error={error}
				isEmpty={roles.length === 0}
				minWidth="min-w-[720px]"
				empty={{
					icon: Users,
					title: "No roles",
					description: "Create one to give an application its own credentials.",
					action: create,
				}}
				columns={
					<>
						<Th>Name</Th>
						<Th>Login</Th>
						<Th>Create DB</Th>
						<Th>Create role</Th>
						<Th>Superuser</Th>
						<Th className="text-right">Connections</Th>
						<ActionsHeader />
					</>
				}
				rows={roles.map((role) => (
					<tr key={role.name}>
						<Td className="font-medium">{role.name}</Td>
						<Td>
							<Flag on={role.canLogin} label="can log in" />
						</Td>
						<Td>
							<Flag on={role.canCreateDatabase} label="can create databases" />
						</Td>
						<Td>
							<Flag on={role.canCreateRole} label="can create roles" />
						</Td>
						<Td>
							<Flag on={role.isSuperuser} label="is a superuser" />
						</Td>
						{/* -1 is PostgreSQL's own way of saying "no limit". */}
						<Td className="text-right text-muted-foreground">
							{role.connectionLimit < 0 ? "unlimited" : role.connectionLimit}
						</Td>
						<Td className="text-right">
							{rowDelete.confirmingId === role.name ? (
								<InlineDeleteConfirm
									label="Drop role?"
									confirmLabel="Drop"
									busy={rowDelete.isDeleting(role.name)}
									onConfirm={() => rowDelete.confirm(role.name, () => dropRole(role.name))}
									onCancel={rowDelete.cancel}
								/>
							) : (
								<IconButton
									variant="danger"
									aria-label={`Drop ${role.name}`}
									onClick={() => rowDelete.ask(role.name)}
								>
									<Trash2 className="h-4 w-4" />
								</IconButton>
							)}
						</Td>
					</tr>
				))}
			/>

			<CreateRolePanel open={creating} onClose={() => setCreating(false)} />
		</>
	);
}
