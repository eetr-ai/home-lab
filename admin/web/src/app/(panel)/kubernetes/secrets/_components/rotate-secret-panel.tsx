"use client";

import { useState } from "react";
import { RefreshCw } from "lucide-react";
import { rotateSecret } from "@/app/actions/kube";
import { Checkbox } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";
import { KeyValueRows } from "./key-value-rows";
import { planRotate, type SecretRow } from "./secret-draft";
import type { SecretSummary } from "@/lib/api/types";

/**
 * Replacing the values of keys a Secret already has.
 *
 * Which keys are on offer comes from the listing rather than from typing, because
 * rotation replaces a value and does not add one — a free-text key here would be
 * a create with weaker guards, and the API refuses it anyway.
 *
 * The keys not chosen keep their values. The panel cannot see them to resend
 * them, so the merge happens in the API against the live object.
 */
export function RotateSecretPanel({
	secret,
	namespace,
	onClose,
}: {
	/** Null when nothing is being rotated, which is also what closes the panel. */
	secret: SecretSummary | null;
	namespace: string;
	onClose: () => void;
}) {
	const [chosen, setChosen] = useState<Record<string, string>>({});

	const rows: SecretRow[] = Object.entries(chosen).map(([key, value]) => ({
		id: key,
		key,
		value,
	}));

	const dirty = Object.keys(chosen).length > 0;

	function reset() {
		setChosen({});
		onClose();
	}

	function toggle(key: string, on: boolean) {
		setChosen((current) => {
			if (on) return { ...current, [key]: "" };
			return Object.fromEntries(Object.entries(current).filter(([held]) => held !== key));
		});
	}

	return (
		<CreatePanel
			open={secret !== null}
			title={secret ? `Rotate ${secret.name}` : "Rotate"}
			icon={RefreshCw}
			submitLabel="Rotate"
			// The thing that is true and easy to miss. Saying it here is the
			// difference between a rotation and a rotation somebody believes took
			// effect — nothing in this panel restarts anything.
			description="Choose the keys to replace; the rest keep their values. This writes the Secret and stops — pods already running hold the old value until something restarts them, which is the Workloads tab."
			dirty={dirty}
			onClose={reset}
			onSubmit={async () => {
				if (!secret) return { ok: false, error: "Nothing to rotate." };
				const plan = planRotate(secret, rows);
				if (!plan.ok) return { ok: false, error: plan.error };
				return rotateSecret(namespace, secret.name, plan.request);
			}}
		>
			<fieldset className="space-y-3">
				<legend className="mb-2 text-sm font-medium">Keys</legend>
				{secret?.keys.map((key) => (
					<Checkbox
						key={key}
						label={key}
						checked={key in chosen}
						onChange={(on) => toggle(key, on)}
					/>
				))}
			</fieldset>

			{rows.length > 0 ? (
				<KeyValueRows
					idPrefix="rotate"
					rows={rows}
					keyOptions={secret?.keys ?? []}
					onChange={(next) =>
						setChosen(Object.fromEntries(next.map((row) => [row.key, row.value])))
					}
				/>
			) : null}
		</CreatePanel>
	);
}
