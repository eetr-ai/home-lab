"use client";

import { useState } from "react";
import { Pencil, Plus } from "lucide-react";
import { editSecret } from "@/app/actions/kube";
import { Button } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";
import { ExistingKeys } from "./existing-keys";
import { KeyValueRows, emptyRow } from "./key-value-rows";
import type { SecretRow } from "./secret-draft";
import {
	actionFor,
	emptyEdit,
	isEditDirty,
	planEdit,
	withAction,
	withReplacedValues,
	type EditDraft,
} from "./secret-edit";
import type { SecretSummary } from "@/lib/api/types";

/**
 * Changing the keys of a Secret that already exists: replacing a value, removing
 * a key, or adding new ones.
 *
 * The keys not touched keep their values. The panel cannot see them to resend
 * them, so the merge happens in the API against the live object — which is also
 * why the existing keys and the new ones are separate lists here. A key typed
 * into "new keys" is never taken to mean one that is already there.
 */
export function EditSecretPanel({
	secret,
	namespace,
	onClose,
}: {
	/** Null when nothing is being edited, which is also what closes the panel. */
	secret: SecretSummary | null;
	namespace: string;
	onClose: () => void;
}) {
	const [draft, setDraft] = useState<EditDraft>(emptyEdit);

	// Ordered by the Secret's own keys rather than by when each was chosen, so the
	// value fields do not rearrange themselves under the operator as they work.
	const replacing: SecretRow[] =
		secret?.keys
			.filter((key) => actionFor(draft, key) === "replace")
			.map((key) => ({ id: key, key, ...draft.existing[key] })) ?? [];

	function reset() {
		setDraft(emptyEdit());
		onClose();
	}

	return (
		<CreatePanel
			open={secret !== null}
			title={secret ? `Edit ${secret.name}` : "Edit Secret"}
			icon={Pencil}
			submitLabel="Save"
			// The thing that is true and easy to miss. Nothing in this panel
			// restarts anything, and an edit somebody believes took effect is worse
			// than one they know is waiting.
			description="Replace a value, remove a key, or add new ones; keys you leave alone keep their values. This writes the Secret and restarts nothing: pods that mount it as an ordinary volume see the change shortly, but ones that read it through environment variables or a subPath mount keep the old values until restarted, which is the Workloads tab."
			dirty={isEditDirty(draft)}
			onClose={reset}
			onSubmit={async () => {
				if (!secret) return { ok: false, error: "Nothing to edit." };
				const plan = planEdit(secret, draft);
				if (!plan.ok) return { ok: false, error: plan.error };
				return editSecret(namespace, secret.name, plan.request);
			}}
		>
			<ExistingKeys
				keys={secret?.keys ?? []}
				actionFor={(key) => actionFor(draft, key)}
				onChange={(key, action) => setDraft((current) => withAction(current, key, action))}
			/>

			{replacing.length > 0 ? (
				<KeyValueRows
					idPrefix="replace"
					rows={replacing}
					keyOptions={secret?.keys ?? []}
					onChange={(rows) => setDraft((current) => withReplacedValues(current, rows))}
				/>
			) : null}

			<fieldset className="space-y-3">
				<legend className="mb-2 text-sm font-medium">New keys</legend>
				{draft.added.length > 0 ? (
					<KeyValueRows
						idPrefix="add"
						rows={draft.added}
						minRows={0}
						onChange={(added) => setDraft((current) => ({ ...current, added }))}
					/>
				) : (
					<Button
						type="button"
						variant="secondary"
						icon={Plus}
						onClick={() => setDraft((current) => ({ ...current, added: [emptyRow()] }))}
					>
						Add a key
					</Button>
				)}
			</fieldset>
		</CreatePanel>
	);
}
