"use client";

import { useState, useTransition } from "react";
import { RotateCcw, Save } from "lucide-react";
import { changeRolePrivileges } from "@/app/actions/postgres";
import { Button } from "@/components/ui";
import type { PostgresObjectPrivileges, PostgresRolePrivileges } from "@/lib/api/types";
import {
	PRIVILEGES,
	baselineDraft,
	countDifferences,
	diffChanges,
	isEditable,
	objectKey,
	togglePrivilege,
	type PrivilegeSection,
} from "@/lib/postgres/privileges";
import { PrivilegeMatrix } from "./privilege-matrix";

const SECTIONS: { section: PrivilegeSection; title: string; qualified: boolean }[] = [
	{ section: "database", title: "Database", qualified: false },
	{ section: "schema", title: "Schemas", qualified: false },
	{ section: "table", title: "Tables and views", qualified: true },
	{ section: "sequence", title: "Sequences", qualified: true },
];

/**
 * The four grids and the one button that applies them.
 *
 * Ticking a box changes a draft, not the database. Applying turns the difference
 * into grouped GRANT and REVOKE statements (see diffChanges) and sends them as one
 * transaction, so a half-applied set of boxes cannot happen: either every change
 * lands, or the server names the one it refused and none do.
 *
 * Keyed by the loaded state upstream, so after a change the refreshed listing
 * starts a fresh draft. That listing is the truth to show — it is read back from
 * the catalog, not inferred from what was asked for.
 */
export function PrivilegeGrid({
	privileges,
	onApplied,
	onError,
}: {
	privileges: PostgresRolePrivileges;
	onApplied: (statements: string[]) => void;
	onError: (message: string | null) => void;
}) {
	const [draft, setDraft] = useState(() => baselineDraft(privileges));
	const [pending, startTransition] = useTransition();
	const differences = countDifferences(privileges, draft);

	const objectsOf = (section: PrivilegeSection): PostgresObjectPrivileges[] => {
		switch (section) {
			case "database":
				return [privileges.databasePrivileges];
			case "schema":
				return privileges.schemas;
			case "table":
				return privileges.tables;
			case "sequence":
				return privileges.sequences;
		}
	};

	function apply() {
		onError(null);
		startTransition(async () => {
			const result = await changeRolePrivileges(privileges.database, privileges.role, diffChanges(privileges, draft));
			if (!result.ok) {
				onError(result.error);
				return;
			}
			onApplied(result.data.statements);
		});
	}

	return (
		<div className="flex flex-col gap-6">
			{SECTIONS.map(({ section, title, qualified }) => (
				<PrivilegeMatrix
					key={section}
					role={privileges.role}
					title={title}
					columns={PRIVILEGES[section]}
					objects={objectsOf(section)}
					qualified={qualified}
					ticked={(object) => draft[objectKey(section, object)] ?? object.granted}
					editable={(object) => isEditable(privileges, object)}
					onToggle={(object, privilege) => setDraft((current) => togglePrivilege(current, section, object, privilege))}
				/>
			))}

			{/* Sticky, because the grid can be long and the button that applies it
			    should not be a scroll away from the box just ticked. */}
			<div className="sticky bottom-0 flex items-center justify-end gap-2 border-t border-border bg-background py-3">
				<span className="mr-auto text-sm text-muted-foreground">
					{differences === 0
						? "No unsaved changes."
						: `${differences} ${differences === 1 ? "box differs" : "boxes differ"} from what is granted.`}
				</span>
				<Button
					variant="secondary"
					icon={RotateCcw}
					disabled={differences === 0 || pending}
					onClick={() => setDraft(baselineDraft(privileges))}
				>
					Reset
				</Button>
				<Button icon={Save} loading={pending} disabled={differences === 0} onClick={apply}>
					Apply
				</Button>
			</div>
		</div>
	);
}
