"use client";

import { useState } from "react";
import { KeyRound } from "lucide-react";
import { putSecret } from "@/app/actions/kube";
import { Checkbox, FormField, Input, Select } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";
import { useNamespaceScope } from "../../../_components/namespace-scope";
import { KeyValueRows, emptyRow } from "./key-value-rows";
import { planCreate, type CreateDraft } from "./secret-draft";

function empty(namespace: string): CreateDraft {
	return { namespace, name: "", rows: [emptyRow()], overwrite: false };
}

/**
 * A Secret is written somewhere, and the somewhere used to be off-screen.
 *
 * The namespace was the page's, named only in this panel's description, and the
 * page had resolved it from a dropdown that quietly defaulted to whichever
 * namespace sorts first. A Secret in the wrong namespace is invisible afterwards:
 * it is a real Secret, in a real namespace, and nothing complains — the release
 * that needed it simply never finds it.
 *
 * So the namespace is a field of its own, above the name, showing where this is
 * going and letting it be changed without leaving the form. It is pre-filled from
 * the page's namespace, and left empty when that is not one the API would write
 * into — an empty required field is a question, where a silently substituted
 * namespace is the mistake this is here to stop.
 */
export function CreateSecretPanel({
	open,
	namespace,
	onClose,
}: {
	open: boolean;
	namespace: string;
	onClose: () => void;
}) {
	const { namespaces } = useNamespaceScope();
	// Only the namespaces the API would write into. The answer is the API's own,
	// carried on every namespace it lists, so this offer and that refusal cannot
	// disagree — a choice that comes back 403 is worse than one never offered.
	const writable = namespaces.filter((one) => one.helmManaged && !one.protected);
	const suggested = writable.some((one) => one.name === namespace) ? namespace : "";

	const [draft, setDraft] = useState<CreateDraft>(() => empty(suggested));

	// The rows carry generated ids, so comparing the whole draft would call every
	// fresh panel dirty. What the operator has actually filled in is the name, the
	// key/value pairs, the namespace if they changed it, and the overwrite box.
	const dirty =
		draft.name !== "" ||
		draft.namespace !== suggested ||
		draft.overwrite ||
		draft.rows.some((row) => row.key !== "" || row.value !== "");

	function reset() {
		setDraft(empty(suggested));
		onClose();
	}

	return (
		<CreatePanel
			open={open}
			title="New Secret"
			icon={KeyRound}
			description="An Opaque Secret. The values are not readable back through this panel afterwards, so take a copy of anything you will need again."
			dirty={dirty}
			onClose={reset}
			onSubmit={async () => {
				const plan = planCreate(draft);
				if (!plan.ok) return { ok: false, error: plan.error };
				return putSecret(plan.namespace, plan.name, plan.request);
			}}
		>
			{/* First, and above the name: it is the field most likely to be wrong
			    and the only one whose being wrong is silent. */}
			<FormField label="Namespace" htmlFor="secret-namespace">
				<Select
					id="secret-namespace"
					value={draft.namespace}
					onChange={(event) => setDraft({ ...draft, namespace: event.target.value })}
					className="w-full"
					required
				>
					<option value="">Choose a namespace</option>
					{writable.map((one) => (
						<option key={one.name} value={one.name}>
							{one.name}
						</option>
					))}
				</Select>
			</FormField>

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
