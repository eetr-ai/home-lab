"use client";

import { useState } from "react";
import { Users } from "lucide-react";
import { updateRole } from "@/app/actions/postgres";
import { FormField, Input } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";
import type { PostgresRole } from "@/lib/api/types";

/** PostgreSQL's own way of saying "no limit". */
const UNLIMITED = -1;

const PRIVILEGES = [
	["canLogin", "Can log in"],
	["canCreateDatabase", "Can create databases"],
	["canCreateRole", "Can create roles"],
] as const;

/**
 * Editing a role.
 *
 * Built on CreatePanel, which is a side panel with an in-flight state, a failure
 * banner and an unsaved-changes guard — none of which differ between creating and
 * editing. Only the submit label does.
 *
 * The form sends the whole desired state rather than a set of changes. ALTER ROLE
 * leaves an unmentioned attribute alone, so the API writes every one explicitly;
 * sending all of them from a form that already shows all of them keeps the two
 * halves saying the same thing.
 */
export function EditRolePanel({ role, onClose }: { role: PostgresRole | null; onClose: () => void }) {
	// Keyed by role upstream, so a new instance starts from that role's values.
	const initial = {
		canLogin: role?.canLogin ?? false,
		canCreateDatabase: role?.canCreateDatabase ?? false,
		canCreateRole: role?.canCreateRole ?? false,
		connectionLimit: String(role?.connectionLimit ?? UNLIMITED),
		password: "",
	};
	const [draft, setDraft] = useState(initial);

	const dirty = JSON.stringify(draft) !== JSON.stringify(initial);
	const limit = Number(draft.connectionLimit);
	const limitValid = /^-?\d+$/.test(draft.connectionLimit.trim()) && limit >= UNLIMITED;

	return (
		<CreatePanel
			open={role !== null}
			title={role ? `Edit ${role.name}` : "Edit role"}
			icon={Users}
			submitLabel="Save"
			description="A new password never reaches PostgreSQL: the API derives a SCRAM-SHA-256 verifier from it and sends that. Leave it empty to keep the existing one."
			dirty={dirty}
			onClose={() => {
				setDraft(initial);
				onClose();
			}}
			onSubmit={async () => {
				if (!role) return { ok: false as const, error: "no role selected" };
				if (!limitValid) {
					return { ok: false as const, error: `a connection limit is a count, or ${UNLIMITED} for unlimited` };
				}
				return updateRole(role.name, {
					canLogin: draft.canLogin,
					canCreateDatabase: draft.canCreateDatabase,
					canCreateRole: draft.canCreateRole,
					connectionLimit: limit,
					...(draft.password ? { password: draft.password } : {}),
				});
			}}
		>
			<fieldset>
				{/* A legend rather than a <label>: this names a group of controls, and a
				    label pointing at nothing is what a screen reader reports it as. */}
				<legend className="mb-1 block text-sm text-muted-foreground">Privileges</legend>
				<div className="space-y-2">
					{PRIVILEGES.map(([key, label]) => (
						<label key={key} className="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								checked={draft[key]}
								onChange={(event) => setDraft({ ...draft, [key]: event.target.checked })}
								className="rounded-chip border-border-strong"
							/>
							{label}
						</label>
					))}
				</div>
			</fieldset>

			<FormField label="Connection limit" htmlFor="role-connections">
				<Input
					id="role-connections"
					type="number"
					min={UNLIMITED}
					value={draft.connectionLimit}
					onChange={(event) => setDraft({ ...draft, connectionLimit: event.target.value })}
					placeholder="-1 for unlimited"
				/>
			</FormField>

			<FormField label="New password" htmlFor="role-new-password">
				<Input
					id="role-new-password"
					type="password"
					value={draft.password}
					onChange={(event) => setDraft({ ...draft, password: event.target.value })}
					autoComplete="new-password"
					placeholder="Leave empty to keep the current one"
				/>
			</FormField>
		</CreatePanel>
	);
}
