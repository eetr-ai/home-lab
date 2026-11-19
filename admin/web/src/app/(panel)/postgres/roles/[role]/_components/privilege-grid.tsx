"use client";

import { useState, useTransition } from "react";
import { RotateCcw, Save } from "lucide-react";
import { changeRolePrivileges } from "@/app/actions/postgres";
import { Button } from "@/components/ui";
import type { PostgresObjectPrivileges, PostgresRolePrivileges } from "@/lib/api/types";
import {
	MAX_CHANGES_PER_REQUEST,
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
		const changes = diffChanges(privileges, draft);
		if (changes.length > MAX_CHANGES_PER_REQUEST) {
			// Explained here rather than sent and refused: the draft is kept, and the
			// operator can apply it in parts or use "Grant a schema" for the bulk.
			onError(
				`These boxes make ${changes.length} separate grants and revokes; one apply can carry at most ${MAX_CHANGES_PER_REQUEST}. Apply part of them first, or use "Grant a schema" for a whole schema.`,
			);
			return;
		}
		onError(null);
		startTransition(async () => {
			// On failure nothing below changes the draft, so the boxes stay as
			// ticked and editable again once pending clears.
			const result = await changeRolePrivileges(privileges.database, privileges.role, changes);
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
					// Locked while applying: an edit made now would not be in the request,
					// and the refresh that follows success would discard it.
					editable={(object) => !pending && isEditable(privileges, object)}
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
