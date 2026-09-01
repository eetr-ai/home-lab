"use client";

import { useState } from "react";
import { Users } from "lucide-react";
import { createRole } from "@/app/actions/postgres";
import { FormField, Input } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";
import { InstallSecretFields } from "../../../_components/install-secret-fields";
import { useCredentialInstall } from "../../../_components/use-credential-install";
import { EMPTY_INSTALL } from "@/lib/secrets/install-draft";
import type { Namespace } from "@/lib/api/types";

const EMPTY = { name: "", password: "", canLogin: true, canCreateDatabase: false, canCreateRole: false };

export function CreateRolePanel({
	open,
	namespaces,
	namespacesError,
	onClose,
}: {
	open: boolean;
	namespaces: Namespace[];
	namespacesError: string | null;
	onClose: () => void;
}) {
	const [draft, setDraft] = useState(EMPTY);
	const secret = useCredentialInstall("role");

	function reset() {
		setDraft(EMPTY);
		secret.reset();
		onClose();
	}

	const dirty =
		JSON.stringify(draft) !== JSON.stringify(EMPTY) ||
		JSON.stringify(secret.install) !== JSON.stringify(EMPTY_INSTALL);

	// A role that cannot log in has a password nothing will ever accept, so
	// installing it as a Secret would produce a credential that looks complete and
	// fails at connection time — in a workload, a long way from this form. Refused
	// before the role is created, so there is nothing half-finished to undo.
	async function submit() {
		if (secret.install.enabled && !draft.canLogin) {
			return {
				ok: false as const,
				error:
					"This role cannot log in, so its password would authenticate nothing. " +
					"Give it the login privilege, or do not install it as a Secret.",
			};
		}

		return secret.submit({ username: draft.name, password: draft.password }, () =>
			createRole({
				name: draft.name,
				...(draft.password ? { password: draft.password } : {}),
				canLogin: draft.canLogin,
				canCreateDatabase: draft.canCreateDatabase,
				canCreateRole: draft.canCreateRole,
			}),
		);
	}

	return (
		<CreatePanel
			open={open}
			title="New role"
			icon={Users}
			description="The password never reaches PostgreSQL: the API derives a SCRAM-SHA-256 verifier from it and sends that instead."
			dirty={dirty}
			onClose={reset}
			onSubmit={submit}
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

			<InstallSecretFields
				draft={secret.install}
				username={draft.name}
				namespaces={namespaces}
				namespacesError={namespacesError}
				onChange={secret.setInstall}
			/>
		</CreatePanel>
	);
}
