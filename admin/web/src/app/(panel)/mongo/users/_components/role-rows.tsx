"use client";

import { Plus, Trash2 } from "lucide-react";
import { Button, IconButton, Select } from "@/components/ui";
import type { MongoRole } from "@/lib/api/types";

/**
 * The roles MongoDB grants on an ordinary database. Deliberately not the
 * cluster-wide ones — `root`, `*AnyDatabase`, `backup`, `restore` — which the API
 * refuses anyway; offering them here would only produce a rejected request and a
 * puzzled operator.
 */
export const GRANTABLE = ["read", "readWrite", "dbAdmin", "dbOwner", "userAdmin"];

/**
 * The repeatable role editor, shared by the create and edit panels.
 *
 * A user's roles are the same thing whether they are being granted for the first
 * time or changed, and MongoDB's updateUser replaces the array outright — so the
 * edit form is the create form with the current roles already in it.
 */
export function RoleRows({
	roles,
	databases,
	database,
	onChange,
}: {
	roles: MongoRole[];
	/** Databases a role may apply to. */
	databases: string[];
	/** The user's own database, used as the default for a newly added role. */
	database: string;
	onChange: (roles: MongoRole[]) => void;
}) {
	function replace(index: number, patch: Partial<MongoRole>) {
		onChange(roles.map((role, at) => (at === index ? { ...role, ...patch } : role)));
	}

	return (
		<fieldset>
			{/* A legend rather than a <label>: this names a group of controls, and a
			    label pointing at nothing is what a screen reader reports it as. */}
			<legend className="mb-1 block text-sm text-muted-foreground">Roles</legend>
			<div className="space-y-2">
				{roles.map((role, index) => (
					// Index as key: these rows have no identity of their own — two
					// identical grants are indistinguishable — and the list is only ever
					// appended to or filtered.
					<div key={index} className="flex items-center gap-2">
						<Select
							aria-label="Role"
							className="flex-1"
							value={role.name}
							onChange={(event) => replace(index, { name: event.target.value })}
						>
							{/* A role the panel does not offer may already be granted. It is
							    listed explicitly rather than being silently swapped for
							    whichever grantable role sorts first. */}
							{GRANTABLE.includes(role.name) ? null : (
								<option value={role.name}>{role.name}</option>
							)}
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
							onChange={(event) => replace(index, { database: event.target.value })}
						>
							{databases.includes(role.database) ? null : (
								<option value={role.database}>{role.database}</option>
							)}
							{databases.map((candidate) => (
								<option key={candidate} value={candidate}>
									{candidate}
								</option>
							))}
						</Select>
						<IconButton
							variant="danger"
							aria-label="Remove role"
							onClick={() => onChange(roles.filter((_, at) => at !== index))}
						>
							<Trash2 className="h-4 w-4" />
						</IconButton>
					</div>
				))}

				<Button
					type="button"
					variant="secondary"
					icon={Plus}
					onClick={() => onChange([...roles, { name: "readWrite", database }])}
				>
					Add role
				</Button>
			</div>
		</fieldset>
	);
}
