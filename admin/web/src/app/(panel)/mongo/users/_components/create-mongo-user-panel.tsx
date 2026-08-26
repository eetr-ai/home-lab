"use client";

import { useState } from "react";
import { UserRound } from "lucide-react";
import { createUser } from "@/app/actions/mongo";
import { FormField, Input } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";
import { RoleRows } from "./role-rows";
import type { MongoRole } from "@/lib/api/types";

export function CreateMongoUserPanel({
	open,
	database,
	databases,
	onClose,
}: {
	open: boolean;
	database: string;
	databases: string[];
	onClose: () => void;
}) {
	const [name, setName] = useState("");
	const [password, setPassword] = useState("");
	const [roles, setRoles] = useState<MongoRole[]>([]);

	function reset() {
		setName("");
		setPassword("");
		setRoles([]);
		onClose();
	}

	return (
		<CreatePanel
			open={open}
			title="New user"
			icon={UserRound}
			description={`Authenticates against ${database || "the selected database"}.`}
			dirty={name !== "" || password !== "" || roles.length > 0}
			onClose={reset}
			onSubmit={() => createUser(database, { name, password, roles })}
		>
			<FormField label="Name" htmlFor="mongo-user-name">
				<Input
					id="mongo-user-name"
					value={name}
					onChange={(event) => setName(event.target.value)}
					placeholder="events_app"
					autoComplete="off"
					required
				/>
			</FormField>

			<FormField label="Password" htmlFor="mongo-user-password">
				<Input
					id="mongo-user-password"
					type="password"
					value={password}
					onChange={(event) => setPassword(event.target.value)}
					autoComplete="new-password"
					required
				/>
			</FormField>

			<RoleRows
				roles={roles}
				databases={databases}
				database={database}
				onChange={setRoles}
			/>
		</CreatePanel>
	);
}
