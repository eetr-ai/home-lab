"use client";

import { FileText, KeyRound, Plus, X, type LucideIcon } from "lucide-react";
import { Button, IconButton, Input, Label, cn, inputClass } from "@/components/ui";
import { SecretInput } from "../../../_components/secret-input";
import { canUseFormat, type SecretFormat, type SecretRow } from "./secret-draft";

/**
 * The key/value rows both Secret forms are made of.
 *
 * Shared because they are the same control in each: a key beside a value, the
 * value being a SecretInput so it can be generated rather than invented. What
 * differs is only whether the key is typed or fixed, which is the `keyOptions`
 * prop — a rotation may only name keys the Secret already has, so there it is a
 * list to pick from rather than a box.
 *
 * Each row also picks how its value is typed: the password field, or a textarea
 * for a value that is a file's contents. See SecretFormat for why that matters.
 *
 * The labels sit in a header line above the rows rather than in the first row,
 * because a textarea makes a row tall and the rows align to their top edge — a
 * label in row one would push its fields down and out of line with the buttons.
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
			<div>
				<div className="flex gap-2">
					<Label htmlFor={rows[0] ? `${idPrefix}-key-${rows[0].id}` : undefined} className="w-1/3">
						Key
					</Label>
					<Label htmlFor={rows[0] ? `${idPrefix}-value-${rows[0].id}` : undefined}>Value</Label>
				</div>

				<div className="space-y-3">
					{rows.map((row, index) => (
						<div key={row.id} className="flex items-start gap-2">
							<div className="w-1/3 min-w-0">
								{keyOptions ? (
									<Input
										id={`${idPrefix}-key-${row.id}`}
										value={row.key}
										readOnly
										aria-label={index === 0 ? undefined : `Key ${index + 1}`}
										className="font-mono"
									/>
								) : (
									<Input
										id={`${idPrefix}-key-${row.id}`}
										value={row.key}
										onChange={(event) => update(row.id, { key: event.target.value })}
										placeholder="password"
										aria-label={index === 0 ? undefined : `Key ${index + 1}`}
										autoComplete="off"
										spellCheck={false}
										className="font-mono"
									/>
								)}
							</div>

							<div className="min-w-0 flex-1">
								{row.format === "file" ? (
									<textarea
										id={`${idPrefix}-value-${row.id}`}
										value={row.value}
										onChange={(event) => update(row.id, { value: event.target.value })}
										aria-label={row.key ? `Contents of ${row.key}` : `Value ${index + 1}`}
										placeholder="Paste the file's contents"
										rows={6}
										// Off, so a long line — a base64 blob, a certificate body —
										// scrolls instead of wrapping into something that looks like
										// line breaks that are not in the value.
										wrap="off"
										autoComplete="off"
										spellCheck={false}
										className={cn(inputClass, "block resize-y font-mono text-sm")}
									/>
								) : (
									<SecretInput
										id={`${idPrefix}-value-${row.id}`}
										value={row.value}
										onChange={(value) => update(row.id, { value })}
										generateLabel={row.key ? `Generate a value for ${row.key}` : "Generate a value"}
									/>
								)}
							</div>

							<FormatToggle
								row={row}
								name={row.key || `row ${index + 1}`}
								onChange={(format) => update(row.id, { format })}
							/>

							{keyOptions ? null : (
								<IconButton
									type="button"
									variant="danger"
									aria-label={`Remove row ${index + 1}`}
									// The last row is not removable: a form with no rows has
									// nothing to type into and no obvious way back.
									disabled={rows.length === 1}
									onClick={() => onChange(rows.filter((other) => other.id !== row.id))}
									// Level with the middle of a single-line field, which is
									// what the row's top edge is.
									className="mt-1"
								>
									<X className="h-4 w-4" />
								</IconButton>
							)}
						</div>
					))}
				</div>
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
	return { id, key: "", value: "", format: "password" };
}

const FORMATS: { format: SecretFormat; label: string; icon: LucideIcon }[] = [
	{ format: "password", label: "Password", icon: KeyRound },
	{ format: "file", label: "File", icon: FileText },
];

/**
 * Password or file, for one row.
 *
 * The same segmented control as the theme switcher, so it reads as a choice
 * between two things rather than as an action. The password side is disabled
 * while the value has line breaks — canUseFormat says why — and its title says
 * so, because a control that is greyed out for no stated reason reads as broken.
 */
function FormatToggle({
	row,
	name,
	onChange,
}: {
	row: SecretRow;
	name: string;
	onChange: (format: SecretFormat) => void;
}) {
	return (
		<div
			role="group"
			aria-label={`How ${name} is entered`}
			className="mt-0.5 inline-flex shrink-0 items-center gap-0.5 rounded-full border border-border p-0.5"
		>
			{FORMATS.map(({ format, label, icon: Icon }) => {
				const active = row.format === format;
				const allowed = canUseFormat(row, format);
				return (
					<button
						key={format}
						type="button"
						onClick={() => onChange(format)}
						disabled={!active && !allowed}
						aria-label={label}
						aria-pressed={active}
						title={
							allowed
								? label
								: `${label} — the value has line breaks a password field would drop`
						}
						className={cn(
							"inline-flex items-center justify-center rounded-full p-1.5 transition-colors disabled:cursor-not-allowed disabled:opacity-40",
							active
								? "bg-surface-sunken text-foreground"
								: "text-muted-foreground enabled:hover:bg-surface-hover enabled:hover:text-foreground",
						)}
					>
						<Icon className="h-4 w-4" />
					</button>
				);
			})}
		</div>
	);
}
