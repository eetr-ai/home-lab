"use client";

import { useState } from "react";
import { Plus, ShieldCheck } from "lucide-react";
import { Banner, Button, EmptyState } from "@/components/ui";
import { ScopePicker } from "../../../../_components/scope-picker";
import type { PostgresRolePrivileges } from "@/lib/api/types";
import { AppliedStatements } from "./applied-statements";
import { DefaultPrivilegesList } from "./default-privileges-list";
import { PrivilegeGrid } from "./privilege-grid";
import { SchemaPresetPanel } from "./schema-preset-panel";

/**
 * A role's privileges in one database: a grid of what it holds, a way to grant a
 * whole schema at once, and the default privileges that cover tables created
 * later.
 *
 * Every change reports the statements it ran. They are the useful record — the
 * same SQL an operator would otherwise have typed into the console — and showing
 * them is what makes a grid of checkboxes trustworthy as a way to run DDL.
 */
export function RolePrivileges({
	role,
	databases,
	selected,
	databaseOwner,
	roles,
	privileges,
	loadError,
}: {
	role: string;
	databases: string[];
	selected: string;
	databaseOwner: string;
	roles: string[];
	privileges: PostgresRolePrivileges | null;
	loadError: string | null;
}) {
	const [error, setError] = useState<string | null>(loadError);
	const [applied, setApplied] = useState<string[]>([]);
	const [presetOpen, setPresetOpen] = useState(false);

	function report(statements: string[]) {
		setError(null);
		setApplied(statements);
	}

	const editable = privileges !== null && !privileges.isSuperuser;

	return (
		<>
			<div className="flex flex-wrap items-start justify-between gap-4">
				<div>
					<h2 className="flex items-center gap-2 text-lg font-medium">
						<ShieldCheck className="h-5 w-5" />
						Privileges of {role}
					</h2>
					<p className="mt-1 text-sm text-muted-foreground">
						Ticked boxes are granted to {role} by name. A muted tick is held another way — through
						PUBLIC or a role it belongs to — and has to be revoked there.
					</p>
				</div>
				{editable ? (
					<Button icon={Plus} onClick={() => setPresetOpen(true)}>
						Grant a schema
					</Button>
				) : null}
			</div>

			<ScopePicker label="Database" param="database" options={databases} selected={selected} />

			<Banner variant="error" message={error} />
			<AppliedStatements statements={applied} onDismiss={() => setApplied([])} />
			{privileges?.isSuperuser ? (
				<Banner
					variant="info"
					message={`${role} is a superuser, which bypasses every privilege check. Granting to it would change nothing, so nothing here can be changed.`}
				/>
			) : null}

			{privileges === null ? (
				error ? null : (
					<EmptyState
						icon={ShieldCheck}
						title="No databases"
						description="Create a database first; privileges are granted per database."
					/>
				)
			) : (
				<>
					<PrivilegeGrid
						// Keyed by what was loaded, so the refresh after a change starts the
						// boxes from the new state instead of the draft that produced it.
						key={JSON.stringify(privileges)}
						privileges={privileges}
						onApplied={report}
						onError={setError}
					/>
					<DefaultPrivilegesList privileges={privileges} onApplied={report} onError={setError} />
					<SchemaPresetPanel
						open={presetOpen}
						privileges={privileges}
						roles={roles}
						databaseOwner={databaseOwner}
						onApplied={report}
						onClose={() => setPresetOpen(false)}
					/>
				</>
			)}
		</>
	);
}
