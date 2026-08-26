"use client";

import { useState } from "react";
import { Database } from "lucide-react";
import { updateDatabase } from "@/app/actions/postgres";
import { FormField, Select } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";
import type { PostgresDatabase } from "@/lib/api/types";

/**
 * Editing a database, which means its owner and nothing else.
 *
 * Encoding cannot be changed after creation, and a rename would break every
 * connection string pointing at the database — not something to offer from a
 * form that has no way to warn about it.
 */
export function EditDatabasePanel({
	database,
	owners,
	onClose,
}: {
	database: PostgresDatabase | null;
	owners: string[];
	onClose: () => void;
}) {
	const initial = database?.owner ?? "";
	const [owner, setOwner] = useState(initial);

	return (
		<CreatePanel
			open={database !== null}
			title={database ? `Edit ${database.name}` : "Edit database"}
			icon={Database}
			submitLabel="Save"
			description="Reassigns the database to another role. Its objects keep their own owners; only the database itself changes hands."
			dirty={owner !== initial}
			onClose={() => {
				setOwner(initial);
				onClose();
			}}
			onSubmit={async () => {
				if (!database) return { ok: false as const, error: "no database selected" };
				return updateDatabase(database.name, { owner });
			}}
		>
			<FormField label="Owner" htmlFor="database-owner">
				<Select
					id="database-owner"
					value={owner}
					onChange={(event) => setOwner(event.target.value)}
					required
				>
					{/* The current owner may not be in the list — it could be a role the
					    panel cannot see — so it is offered explicitly rather than being
					    silently replaced by whichever role happens to sort first. */}
					{owners.includes(initial) ? null : <option value={initial}>{initial}</option>}
					{owners.map((name) => (
						<option key={name} value={name}>
							{name}
						</option>
					))}
				</Select>
			</FormField>
		</CreatePanel>
	);
}
