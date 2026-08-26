"use client";

import { useState } from "react";
import { Pencil, Plus, Trash2, UserRound } from "lucide-react";
import { dropUser } from "@/app/actions/mongo";
import { Button, IconButton, InlineDeleteConfirm, Td, Th } from "@/components/ui";
import { ActionsHeader, Directory } from "../../../_components/directory";
import { ScopePicker } from "../../../_components/scope-picker";
import { useRowDelete } from "../../../_components/use-row-delete";
import type { MongoUser } from "@/lib/api/types";
import { CreateMongoUserPanel } from "./create-mongo-user-panel";
import { EditMongoUserPanel } from "./edit-mongo-user-panel";

export function MongoUserList({
	databases,
	selected,
	users,
	loadError,
}: {
	databases: string[];
	selected: string;
	users: MongoUser[];
	loadError: string | null;
}) {
	const [error, setError] = useState<string | null>(loadError);
	const [creating, setCreating] = useState(false);
	const [editing, setEditing] = useState<MongoUser | null>(null);
	const rowDelete = useRowDelete(setError);

	const create = (
		<Button icon={Plus} onClick={() => setCreating(true)} disabled={!selected}>
			New user
		</Button>
	);

	return (
		<>
			<div className="flex flex-wrap items-start justify-between gap-3">
				<ScopePicker label="Database" param="database" options={databases} selected={selected} />
				<div className="mb-4">{create}</div>
			</div>

			<Directory
				error={error}
				isEmpty={users.length === 0}
				minWidth="min-w-[640px]"
				empty={{
					icon: UserRound,
					title: selected ? "No users" : "No databases",
					description: selected
						? "Create one to give an application credentials scoped to this database."
						: "Create a database first.",
					action: selected ? create : undefined,
				}}
				columns={
					<>
						<Th>Name</Th>
						<Th>Roles</Th>
						<ActionsHeader />
					</>
				}
				rows={users.map((user) => (
					<tr key={user.name}>
						<Td className="font-medium">{user.name}</Td>
						{/* Roles name their own database, which is not always this one — a
						    role granted on `admin` reaches every database. Showing the pair
						    is the only way to tell those apart. */}
						<Td className="text-muted-foreground">
							{user.roles.map((role) => `${role.name}@${role.database}`).join(", ") || "—"}
						</Td>
						<Td className="text-right">
							{rowDelete.confirmingId === user.name ? (
								<InlineDeleteConfirm
									label="Drop user?"
									confirmLabel="Drop"
									busy={rowDelete.isDeleting(user.name)}
									onConfirm={() =>
										rowDelete.confirm(user.name, () => dropUser(selected, user.name))
									}
									onCancel={rowDelete.cancel}
								/>
							) : (
								/* Pencil then Trash2, per the UX guidelines' row-action order. */
								<div className="flex items-center justify-end gap-1">
									<IconButton aria-label={`Edit ${user.name}`} onClick={() => setEditing(user)}>
										<Pencil className="h-4 w-4" />
									</IconButton>
									<IconButton
										variant="danger"
										aria-label={`Drop ${user.name}`}
										onClick={() => rowDelete.ask(user.name)}
									>
										<Trash2 className="h-4 w-4" />
									</IconButton>
								</div>
							)}
						</Td>
					</tr>
				))}
			/>

			<CreateMongoUserPanel
				open={creating}
				database={selected}
				databases={databases}
				onClose={() => setCreating(false)}
			/>
			{/* Keyed by user, so opening a different one starts from its own roles
			    rather than from whatever was last edited. */}
			<EditMongoUserPanel
				key={editing?.name}
				user={editing}
				databases={databases}
				onClose={() => setEditing(null)}
			/>
		</>
	);
}
