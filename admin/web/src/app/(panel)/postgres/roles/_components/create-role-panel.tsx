"use client";

import { useState } from "react";
import { Users } from "lucide-react";
import { createRole } from "@/app/actions/postgres";
import { FormField, Input } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";

const EMPTY = { name: "", password: "", canLogin: true, canCreateDatabase: false, canCreateRole: false };

export function CreateRolePanel({ open, onClose }: { open: boolean; onClose: () => void }) {
	const [draft, setDraft] = useState(EMPTY);

	function reset() {
		setDraft(EMPTY);
		onClose();
	}

	const dirty = JSON.stringify(draft) !== JSON.stringify(EMPTY);

	return (
		<CreatePanel
			open={open}
			title="New role"
			icon={Users}
			description="The password never reaches PostgreSQL: the API derives a SCRAM-SHA-256 verifier from it and sends that instead."
			dirty={dirty}
			onClose={reset}
			onSubmit={() =>
				createRole({
					name: draft.name,
					...(draft.password ? { password: draft.password } : {}),
					canLogin: draft.canLogin,
					canCreateDatabase: draft.canCreateDatabase,
					canCreateRole: draft.canCreateRole,
				})
			}
		>
			<FormField label="Name" htmlFor="role-name">
				<Input
					id="role-name"
					value={draft.name}
					onChange={(event) => setDraft({ ...draft, name: event.target.value })}
					placeholder="analytics_app"
					autoComplete="off"
					required
				/>
			</FormField>

			<FormField label="Password" htmlFor="role-password">
				<Input
					id="role-password"
					type="password"
					value={draft.password}
					onChange={(event) => setDraft({ ...draft, password: event.target.value })}
					autoComplete="new-password"
					placeholder="Leave empty for a role that cannot authenticate"
				/>
			</FormField>

			<fieldset>
				{/* A legend rather than a <label>: this names a group of controls, and a
				    label pointing at nothing is what a screen reader reports it as. */}
				<legend className="mb-1 block text-sm text-muted-foreground">Privileges</legend>
				<div className="space-y-2">
					{(
						[
							["canLogin", "Can log in"],
							["canCreateDatabase", "Can create databases"],
							["canCreateRole", "Can create roles"],
						] as const
					).map(([key, label]) => (
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
		</CreatePanel>
	);
}
