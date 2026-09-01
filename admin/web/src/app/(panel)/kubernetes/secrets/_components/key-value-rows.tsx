"use client";

import { Plus, X } from "lucide-react";
import { Button, IconButton, Input, Label } from "@/components/ui";
import { SecretInput } from "../../../_components/secret-input";
import type { SecretRow } from "./secret-draft";

/**
 * The key/value rows both Secret forms are made of.
 *
 * Shared because they are the same control in each: a key beside a value, the
 * value being a SecretInput so it can be generated rather than invented. What
 * differs is only whether the key is typed or fixed, which is the `keyOptions`
 * prop — a rotation may only name keys the Secret already has, so there it is a
 * list to pick from rather than a box.
 */
export function KeyValueRows({
	rows,
	onChange,
	keyOptions,
	idPrefix,
}: {
	rows: SecretRow[];
	onChange: (rows: SecretRow[]) => void;
	/** When set, the key is one of these rather than free text. */
	keyOptions?: string[];
	idPrefix: string;
}) {
	function update(id: string, patch: Partial<SecretRow>) {
		onChange(rows.map((row) => (row.id === id ? { ...row, ...patch } : row)));
	}

	return (
		<div className="space-y-3">
			<div className="space-y-3">
				{rows.map((row, index) => (
					<div key={row.id} className="space-y-1">
						<div className="flex items-end gap-2">
							<div className="w-1/3 min-w-0 space-y-1">
								<Label htmlFor={`${idPrefix}-key-${row.id}`}>{index === 0 ? "Key" : ""}</Label>
								{keyOptions ? (
									<Input
										id={`${idPrefix}-key-${row.id}`}
										value={row.key}
										readOnly
										className="font-mono"
									/>
								) : (
									<Input
										id={`${idPrefix}-key-${row.id}`}
										value={row.key}
										onChange={(event) => update(row.id, { key: event.target.value })}
										placeholder="password"
										autoComplete="off"
										spellCheck={false}
										className="font-mono"
									/>
								)}
							</div>

							<div className="min-w-0 flex-1 space-y-1">
								<Label htmlFor={`${idPrefix}-value-${row.id}`}>{index === 0 ? "Value" : ""}</Label>
								<SecretInput
									id={`${idPrefix}-value-${row.id}`}
									value={row.value}
									onChange={(value) => update(row.id, { value })}
									generateLabel={row.key ? `Generate a value for ${row.key}` : "Generate a value"}
								/>
							</div>

							{keyOptions ? null : (
								<IconButton
									type="button"
									variant="danger"
									aria-label={`Remove row ${index + 1}`}
									// The last row is not removable: a form with no rows has
									// nothing to type into and no obvious way back.
									disabled={rows.length === 1}
									onClick={() => onChange(rows.filter((other) => other.id !== row.id))}
								>
									<X className="h-4 w-4" />
								</IconButton>
							)}
						</div>
					</div>
				))}
			</div>

			{keyOptions ? null : (
				<Button
					type="button"
					variant="secondary"
					icon={Plus}
					onClick={() => onChange([...rows, emptyRow()])}
				>
					Add a key
				</Button>
			)}
		</div>
	);
}

/**
 * A blank row.
 *
 * The id is only a React key. It never reaches the API — the payload is keyed by
 * the key name — so `randomId`'s reasoning applies: this is not a security
 * boundary and the fallbacks only need to not collide.
 */
export function emptyRow(): SecretRow {
	const c = globalThis.crypto;
	const id =
		typeof c?.randomUUID === "function"
			? c.randomUUID()
			: `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
	return { id, key: "", value: "" };
}
