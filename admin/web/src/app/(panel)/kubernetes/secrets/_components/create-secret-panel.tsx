"use client";

import { useState } from "react";
import { KeyRound } from "lucide-react";
import { putSecret } from "@/app/actions/kube";
import { Checkbox, FormField, Input } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";
import { KeyValueRows, emptyRow } from "./key-value-rows";
import { planCreate, type CreateDraft } from "./secret-draft";

function empty(): CreateDraft {
	return { name: "", rows: [emptyRow()], overwrite: false };
}

export function CreateSecretPanel({
	open,
	namespace,
	onClose,
}: {
	open: boolean;
	namespace: string;
	onClose: () => void;
}) {
	const [draft, setDraft] = useState<CreateDraft>(empty);

	// The rows carry generated ids, so comparing the whole draft would call every
	// fresh panel dirty. What the operator has actually filled in is the name, the
	// key/value pairs, and the overwrite box.
	const dirty =
		draft.name !== "" ||
		draft.overwrite ||
		draft.rows.some((row) => row.key !== "" || row.value !== "");

	function reset() {
		setDraft(empty());
		onClose();
	}

	return (
		<CreatePanel
			open={open}
			title="New Secret"
			icon={KeyRound}
			description={`An Opaque Secret in ${namespace}. The values are not readable back through this panel afterwards, so take a copy of anything you will need again.`}
			dirty={dirty}
			onClose={reset}
			onSubmit={async () => {
				const plan = planCreate(draft);
				if (!plan.ok) return { ok: false, error: plan.error };
				return putSecret(namespace, plan.name, plan.request);
			}}
		>
			<FormField label="Name" htmlFor="secret-name">
				<Input
					id="secret-name"
					value={draft.name}
					onChange={(event) => setDraft({ ...draft, name: event.target.value })}
					placeholder="octo-database"
					autoComplete="off"
					spellCheck={false}
					required
				/>
			</FormField>

			<KeyValueRows
				idPrefix="secret"
				rows={draft.rows}
				onChange={(rows) => setDraft({ ...draft, rows })}
			/>

			<Checkbox
				label="Replace a Secret that is already there"
				hint="Off, a Secret of that name is left alone and the write is refused. On, whatever a running release is using is overwritten."
				checked={draft.overwrite}
				onChange={(overwrite) => setDraft({ ...draft, overwrite })}
			/>
		</CreatePanel>
	);
}
