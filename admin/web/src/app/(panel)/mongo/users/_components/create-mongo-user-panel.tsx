"use client";

import { useState } from "react";
import { Plus, Trash2, UserRound } from "lucide-react";
import { createUser } from "@/app/actions/mongo";
import { Button, FormField, IconButton, Input, Select } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";
import type { MongoRole } from "@/lib/api/types";

/**
 * The roles MongoDB grants on an ordinary database. Deliberately not the
 * cluster-wide ones — `root`, `*AnyDatabase`, `backup`, `restore` — which the API
 * refuses anyway; offering them here would only produce a rejected request and a
 * puzzled operator.
 */
const GRANTABLE = ["read", "readWrite", "dbAdmin", "dbOwner", "userAdmin"];

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

			<fieldset>
				<legend className="mb-1 block text-sm text-muted-foreground">Roles</legend>
				<div className="space-y-2">
					{roles.map((role, index) => (
						<div key={index} className="flex items-center gap-2">
							<Select
								aria-label="Role"
								className="flex-1"
								value={role.name}
								onChange={(event) =>
									setRoles(
										roles.map((current, at) =>
											at === index ? { ...current, name: event.target.value } : current,
										),
									)
								}
							>
								{GRANTABLE.map((grantable) => (
									<option key={grantable} value={grantable}>
										{grantable}
									</option>
								))}
							</Select>
							<Select
								aria-label="On database"
								className="flex-1"
								value={role.database}
								onChange={(event) =>
									setRoles(
										roles.map((current, at) =>
											at === index ? { ...current, database: event.target.value } : current,
										),
									)
								}
							>
								{databases.map((candidate) => (
									<option key={candidate} value={candidate}>
										{candidate}
									</option>
								))}
							</Select>
							<IconButton
								variant="danger"
								aria-label="Remove role"
								onClick={() => setRoles(roles.filter((_, at) => at !== index))}
							>
								<Trash2 className="h-4 w-4" />
							</IconButton>
						</div>
					))}

					<Button
						type="button"
						variant="secondary"
						icon={Plus}
						onClick={() => setRoles([...roles, { name: "readWrite", database }])}
					>
						Add role
					</Button>
				</div>
			</fieldset>
		</CreatePanel>
	);
}
