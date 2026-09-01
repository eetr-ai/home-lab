"use client";

import { useState } from "react";
import { UserRound } from "lucide-react";
import { createUser } from "@/app/actions/mongo";
import { FormField, Input } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";
import { InstallSecretFields } from "../../../_components/install-secret-fields";
import { useCredentialInstall } from "../../../_components/use-credential-install";
import { EMPTY_INSTALL } from "@/lib/secrets/install-draft";
import { RoleRows } from "./role-rows";
import type { MongoRole, Namespace } from "@/lib/api/types";

export function CreateMongoUserPanel({
	open,
	database,
	databases,
	namespaces,
	namespacesError,
	onClose,
}: {
	open: boolean;
	database: string;
	databases: string[];
	namespaces: Namespace[];
	namespacesError: string | null;
	onClose: () => void;
}) {
	const [name, setName] = useState("");
	const [password, setPassword] = useState("");
	const [roles, setRoles] = useState<MongoRole[]>([]);
	const secret = useCredentialInstall("user");

	function reset() {
		setName("");
		setPassword("");
		setRoles([]);
		secret.reset();
		onClose();
	}

	return (
		<CreatePanel
			open={open}
			title="New user"
			icon={UserRound}
			description={`Authenticates against ${database || "the selected database"}.`}
			dirty={
				name !== "" ||
				password !== "" ||
				roles.length > 0 ||
				JSON.stringify(secret.install) !== JSON.stringify(EMPTY_INSTALL)
			}
			onClose={reset}
			onSubmit={() =>
				secret.submit({ username: name, password, database }, () =>
					createUser(database, { name, password, roles }),
				)
			}
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

			<InstallSecretFields
				draft={secret.install}
				username={name}
				database={database}
				namespaces={namespaces}
				namespacesError={namespacesError}
				onChange={secret.setInstall}
			/>
		</CreatePanel>
	);
}
