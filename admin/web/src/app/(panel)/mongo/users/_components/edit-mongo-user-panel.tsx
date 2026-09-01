"use client";

import { useState } from "react";
import { UserRound } from "lucide-react";
import { updateUser } from "@/app/actions/mongo";
import { FormField } from "@/components/ui";
import { SecretInput } from "../../../_components/secret-input";
import { CreatePanel } from "../../../_components/create-panel";
import { RoleRows } from "./role-rows";
import type { MongoRole, MongoUser } from "@/lib/api/types";

/**
 * Editing a user's roles, and optionally its password.
 *
 * The whole role set is sent rather than a set of grants and revocations, because
 * MongoDB's updateUser replaces the array outright. The form starts from the
 * user's current roles, so what is submitted is what is on screen.
 */
export function EditMongoUserPanel({
	user,
	databases,
	onClose,
}: {
	user: MongoUser | null;
	databases: string[];
	onClose: () => void;
}) {
	// Keyed by user upstream, so a new instance starts from that user's roles.
	const initial: MongoRole[] = user?.roles ?? [];
	const [roles, setRoles] = useState(initial);
	const [password, setPassword] = useState("");

	const dirty = password !== "" || JSON.stringify(roles) !== JSON.stringify(initial);

	return (
		<CreatePanel
			open={user !== null}
			title={user ? `Edit ${user.name}` : "Edit user"}
			icon={UserRound}
			submitLabel="Save"
			description="Replaces the user's roles outright — what is listed here is what it will have. Leave the password empty to keep the existing one."
			dirty={dirty}
			onClose={() => {
				setRoles(initial);
				setPassword("");
				onClose();
			}}
			onSubmit={async () => {
				if (!user) return { ok: false as const, error: "no user selected" };
				return updateUser(user.database, user.name, {
					roles,
					...(password ? { password } : {}),
				});
			}}
		>
			<RoleRows
				roles={roles}
				databases={databases}
				database={user?.database ?? ""}
				onChange={setRoles}
			/>

			<FormField label="New password" htmlFor="mongo-user-new-password">
				<SecretInput
					id="mongo-user-new-password"
					value={password}
					onChange={setPassword}
					generateLabel="Generate a password"
					placeholder="Leave empty to keep the current one"
				/>
			</FormField>
		</CreatePanel>
	);
}
