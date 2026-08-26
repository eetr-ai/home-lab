"use client";

import { useState } from "react";
import { Database } from "lucide-react";
import { FormField, Input, Select } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";
import type { ActionResult } from "@/lib/api/result";
import type { CreatePostgresDatabase, PostgresDatabase } from "@/lib/api/types";

export function CreateDatabasePanel({
	open,
	owners,
	onClose,
	onSubmit,
}: {
	open: boolean;
	owners: string[];
	onClose: () => void;
	onSubmit: (request: CreatePostgresDatabase) => Promise<ActionResult<PostgresDatabase>>;
}) {
	const [name, setName] = useState("");
	const [owner, setOwner] = useState("");

	function reset() {
		setName("");
		setOwner("");
		onClose();
	}

	return (
		<CreatePanel
			open={open}
			title="New database"
			icon={Database}
			description="The name is validated against PostgreSQL's identifier rules by the API."
			dirty={name !== "" || owner !== ""}
			onClose={reset}
			onSubmit={() => onSubmit({ name, ...(owner ? { owner } : {}) })}
		>
			<FormField label="Name" htmlFor="database-name">
				<Input
					id="database-name"
					value={name}
					onChange={(event) => setName(event.target.value)}
					placeholder="analytics"
					autoComplete="off"
					required
				/>
			</FormField>

			<FormField label="Owner" htmlFor="database-owner">
				<Select
					id="database-owner"
					className="w-full"
					value={owner}
					onChange={(event) => setOwner(event.target.value)}
				>
					{/* Empty means "do not say", and PostgreSQL then makes the connecting
					    superuser the owner. Spelling that out beats an invisible default. */}
					<option value="">The connecting superuser</option>
					{owners.map((candidate) => (
						<option key={candidate} value={candidate}>
							{candidate}
						</option>
					))}
				</Select>
			</FormField>
		</CreatePanel>
	);
}
