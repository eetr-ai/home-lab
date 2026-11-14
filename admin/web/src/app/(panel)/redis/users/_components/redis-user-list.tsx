"use client";

import { useState } from "react";
import { Pencil, Plus, Trash2, UserRound } from "lucide-react";
import { deleteUser } from "@/app/actions/redis";
import { Banner, Button, IconButton, InlineDeleteConfirm, Td, Th } from "@/components/ui";
import { describeUser } from "@/lib/redis/users";
import type { Namespace } from "@/lib/api/types";
import type { RedisUser } from "@/lib/api/redis-types";
import { ActionsHeader, Directory } from "../../../_components/directory";
import { InteractiveRow, stopRowActivation } from "../../../_components/interactive-row";
import { useRowDelete } from "../../../_components/use-row-delete";
import { CreateRedisUserPanel } from "./create-redis-user-panel";
import { EditRedisUserPanel } from "./edit-redis-user-panel";

export function RedisUserList({
	users,
	persistent,
	loadError,
	namespaces,
	namespacesError,
}: {
	users: RedisUser[];
	persistent: boolean;
	loadError: string | null;
	namespaces: Namespace[];
	namespacesError: string | null;
}) {
	const [error, setError] = useState<string | null>(loadError);
	const [creating, setCreating] = useState(false);
	const [editing, setEditing] = useState<RedisUser | null>(null);
	const rowDelete = useRowDelete(setError);

	const create = (
		<Button icon={Plus} onClick={() => setCreating(true)} disabled={!persistent}>
			New user
		</Button>
	);

	return (
		<>
			<div className="mb-4 flex justify-end">{create}</div>

			{/* The API refuses the writes in this state anyway. Saying why up front
			    beats a disabled button nobody can explain. */}
			{persistent ? null : (
				<Banner
					variant="warning"
					message="This Redis was started without an aclfile, so a user created now would be gone after its next restart. Upgrade the platform chart, then come back."
				/>
			)}

			<Directory
				error={error}
				isEmpty={users.length === 0}
				minWidth="min-w-[640px]"
				empty={{ icon: UserRound, title: "No users", description: "Redis could not list its users." }}
				columns={
					<>
						<Th>Name</Th>
						<Th>May do</Th>
						<Th>Status</Th>
						<ActionsHeader />
					</>
				}
				rows={users.map((user) => {
					const cells = (
						<>
							<Td className="font-medium group-hover:underline">{user.name}</Td>
							<Td className="text-muted-foreground">{describeUser(user)}</Td>
							<Td className="text-muted-foreground">{status(user)}</Td>
							<Td className="text-right" onClick={stopRowActivation}>
								<Actions user={user} rowDelete={rowDelete} onEdit={() => setEditing(user)} />
							</Td>
						</>
					);
					// Only a user the form can express opens the form. A row that looks
					// clickable and is not would be worse than one that plainly is not.
					return user.editable && persistent ? (
						<InteractiveRow key={user.name} onActivate={() => setEditing(user)}>
							{cells}
						</InteractiveRow>
					) : (
						<tr key={user.name}>{cells}</tr>
					);
				})}
			/>

			<CreateRedisUserPanel
				open={creating}
				namespaces={namespaces}
				namespacesError={namespacesError}
				onClose={() => setCreating(false)}
			/>
			{/* Keyed by user, so opening a different one starts from its own state. */}
			<EditRedisUserPanel key={editing?.name} user={editing} onClose={() => setEditing(null)} />
		</>
	);
}

function status(user: RedisUser): string {
	if (!user.enabled) return "Disabled";
	return user.hasPassword ? "Enabled" : "Enabled, no password";
}

function Actions({
	user,
	rowDelete,
	onEdit,
}: {
	user: RedisUser;
	rowDelete: ReturnType<typeof useRowDelete>;
	onEdit: () => void;
}) {
	// The default user is the platform's own credential: its password is a
	// Secret and the panel itself signs in with it. Nothing to offer here.
	if (user.name === "default") return null;

	if (rowDelete.confirmingId === user.name) {
		return (
			<InlineDeleteConfirm
				label="Delete user?"
				busy={rowDelete.isDeleting(user.name)}
				onConfirm={() => rowDelete.confirm(user.name, () => deleteUser(user.name))}
				onCancel={rowDelete.cancel}
			/>
		);
	}

	return (
		<div className="flex items-center justify-end gap-1">
			{user.editable ? (
				<IconButton aria-label={`Edit ${user.name}`} onClick={onEdit}>
					<Pencil className="h-4 w-4" />
				</IconButton>
			) : null}
			<IconButton variant="danger" aria-label={`Delete ${user.name}`} onClick={() => rowDelete.ask(user.name)}>
				<Trash2 className="h-4 w-4" />
			</IconButton>
		</div>
	);
}
