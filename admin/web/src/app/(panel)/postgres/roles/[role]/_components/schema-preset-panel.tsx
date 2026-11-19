"use client";

import { useState } from "react";
import { ShieldCheck } from "lucide-react";
import { changeRolePrivileges } from "@/app/actions/postgres";
import { Checkbox, FormField, Select } from "@/components/ui";
import type { PostgresRolePrivileges } from "@/lib/api/types";
import { PRESET_LABELS, presetChanges, type SchemaPreset } from "@/lib/postgres/privileges";
import { CreatePanel } from "../../../../_components/create-panel";

const PRESET_HINTS: Record<SchemaPreset, string> = {
	readOnly: "SELECT on every table and sequence.",
	readWrite: "SELECT, INSERT, UPDATE and DELETE on every table; USAGE and SELECT on every sequence, which inserting into a serial column needs.",
};

/**
 * Grant a whole schema at once: the shortcut for "give this application role
 * access to its tables", which is otherwise a dozen boxes and an easily forgotten
 * USAGE on the sequences.
 *
 * It covers the tables that exist now. Ticking "created later" adds default
 * privileges as well, and names the owner they apply to, because that is the
 * part that surprises people: default privileges cover only objects created by
 * that one role. It defaults to the database's owner, which is usually the role
 * an application's migrations run as — usually, not always, so it is a field.
 */
export function SchemaPresetPanel({
	open,
	privileges,
	roles,
	databaseOwner,
	onApplied,
	onClose,
}: {
	open: boolean;
	privileges: PostgresRolePrivileges;
	roles: string[];
	databaseOwner: string;
	onApplied: (statements: string[]) => void;
	onClose: () => void;
}) {
	const schemas = privileges.schemas.map((one) => one.name);
	// PostgreSQL's predefined roles own nothing anybody migrates into a schema.
	const owners = roles.filter((role) => !role.startsWith("pg_"));
	const initial = {
		schema: schemas.includes("public") ? "public" : (schemas[0] ?? ""),
		preset: "readWrite" as SchemaPreset,
		future: true,
		owner: owners.includes(databaseOwner) ? databaseOwner : (owners[0] ?? ""),
	};
	const [draft, setDraft] = useState(initial);

	return (
		<CreatePanel
			open={open}
			title={`Grant a schema to ${privileges.role}`}
			icon={ShieldCheck}
			submitLabel="Grant"
			description={`In ${privileges.database}. Applied in one transaction, together with CONNECT on the database and USAGE on the schema.`}
			dirty={JSON.stringify(draft) !== JSON.stringify(initial)}
			onClose={() => {
				setDraft(initial);
				onClose();
			}}
			onSubmit={async () => {
				if (!draft.schema) return { ok: false as const, error: "choose a schema" };
				// Checked rather than left to presetChanges, which drops the default
				// privileges when there is no owner: the grant would then succeed
				// without the part this box asked for.
				if (draft.future && !owners.includes(draft.owner)) {
					return {
						ok: false as const,
						error: "choose who creates the later tables, or untick that option; the roles could not be listed",
					};
				}
				const result = await changeRolePrivileges(
					privileges.database,
					privileges.role,
					presetChanges(draft.preset, draft.schema, draft.future ? draft.owner : null),
				);
				if (result.ok) onApplied(result.data.statements);
				return result;
			}}
		>
			<FormField label="Schema" htmlFor="preset-schema">
				<Select
					id="preset-schema"
					value={draft.schema}
					onChange={(event) => setDraft({ ...draft, schema: event.target.value })}
					className="w-full"
				>
					{schemas.map((schema) => (
						<option key={schema} value={schema}>
							{schema}
						</option>
					))}
				</Select>
			</FormField>

			<fieldset>
				<legend className="mb-1 block text-sm text-muted-foreground">Access</legend>
				<div className="space-y-2">
					{(Object.keys(PRESET_LABELS) as SchemaPreset[]).map((preset) => (
						<label key={preset} className="flex cursor-pointer items-start gap-2.5 text-sm">
							<input
								type="radio"
								name="preset"
								checked={draft.preset === preset}
								onChange={() => setDraft({ ...draft, preset })}
								className="mt-0.5 h-4 w-4 accent-brand"
							/>
							<span>
								<span className="text-foreground">{PRESET_LABELS[preset]}</span>
								<span className="mt-0.5 block text-xs text-muted-foreground">{PRESET_HINTS[preset]}</span>
							</span>
						</label>
					))}
				</div>
			</fieldset>

			<Checkbox
				label="Also tables and sequences created later"
				hint="Default privileges. They apply only to objects created by the owner below; a table any other role creates is not covered."
				checked={draft.future}
				onChange={(future) => setDraft({ ...draft, future })}
			/>

			{draft.future ? (
				<FormField label="Created by" htmlFor="preset-owner">
					<Select
						id="preset-owner"
						value={draft.owner}
						onChange={(event) => setDraft({ ...draft, owner: event.target.value })}
						className="w-full"
					>
						{owners.map((role) => (
							<option key={role} value={role}>
								{role}
								{role === databaseOwner ? " (owns the database)" : ""}
							</option>
						))}
					</Select>
				</FormField>
			) : null}
		</CreatePanel>
	);
}
