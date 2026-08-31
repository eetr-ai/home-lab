"use client";

import { Checkbox, FormField, Input, Select } from "@/components/ui";
import { secretName, type InstallDraft } from "@/lib/secrets/install-draft";
import type { Namespace } from "@/lib/api/types";

/**
 * The "also install this as a Secret" section of a create-credential form.
 *
 * Shared by the PostgreSQL role panel and the MongoDB user panel because it is
 * the same question in both: the credential being created has to reach the chart
 * that will use it, and that chart decides what the keys are called.
 *
 * It renders a draft and reports edits. Every rule about that draft — what is
 * required, what the Secret is called by default, whether two fields collide —
 * lives in lib/secrets/install-draft.ts, where it can be tested without React.
 */
export function InstallSecretFields({
	draft,
	username,
	namespaces,
	namespacesError,
	onChange,
}: {
	draft: InstallDraft;
	/** The credential being created, which is what the Secret is named after. */
	username: string;
	namespaces: Namespace[];
	namespacesError: string | null;
	onChange: (draft: InstallDraft) => void;
}) {
	function set<K extends keyof InstallDraft>(key: K, value: InstallDraft[K]) {
		onChange({ ...draft, [key]: value });
	}

	return (
		<fieldset className="space-y-3 border-t border-border pt-4">
			<Checkbox
				label="Also install this as a Secret"
				hint="Writes the credential into a namespace the panel manages, so a chart can read it with existingSecret."
				checked={draft.enabled}
				onChange={(checked) => set("enabled", checked)}
			/>

			{draft.enabled ? (
				<div className="space-y-3 pl-6">
					<FormField label="Namespace" htmlFor="secret-namespace">
						<Select
							id="secret-namespace"
							className="w-full"
							value={draft.namespace}
							onChange={(event) => set("namespace", event.target.value)}
						>
							<option value="">Choose a namespace</option>
							{namespaces.map((namespace) => (
								<option key={namespace.name} value={namespace.name}>
									{namespace.name}
								</option>
							))}
						</Select>
					</FormField>

					{namespacesError ? (
						<p className="text-xs text-danger">The namespaces could not be read: {namespacesError}</p>
					) : null}

					<FormField label="Secret name" htmlFor="secret-name">
						<Input
							id="secret-name"
							value={draft.name}
							onChange={(event) => set("name", event.target.value)}
							placeholder={secretName({ ...draft, name: "" }, username) || "octo-credentials"}
							autoComplete="off"
						/>
					</FormField>

					{/* The key names are the chart's, not ours: existingSecretPasswordKey
					    and its neighbours differ from chart to chart, so they are fields
					    rather than constants. Clearing one leaves that value out. */}
					<div className="grid grid-cols-2 gap-3">
						<FormField label="Username key" htmlFor="secret-username-key">
							<Input
								id="secret-username-key"
								value={draft.usernameKey}
								onChange={(event) => set("usernameKey", event.target.value)}
								autoComplete="off"
							/>
						</FormField>
						<FormField label="Password key" htmlFor="secret-password-key">
							<Input
								id="secret-password-key"
								value={draft.passwordKey}
								onChange={(event) => set("passwordKey", event.target.value)}
								autoComplete="off"
								required
							/>
						</FormField>
					</div>

					<Checkbox
						label="Replace a Secret that is already there"
						hint="Off, a Secret of that name is left alone and the write is refused. On, whatever a running release is using is overwritten."
						checked={draft.overwrite}
						onChange={(checked) => set("overwrite", checked)}
					/>
				</div>
			) : null}
		</fieldset>
	);
}
