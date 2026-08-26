"use client";

import { useState } from "react";
import { Leaf } from "lucide-react";
import { createDatabase } from "@/app/actions/mongo";
import { FormField, Input } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";

export function CreateMongoDatabasePanel({
	open,
	onClose,
}: {
	open: boolean;
	onClose: () => void;
}) {
	const [name, setName] = useState("");
	const [collection, setCollection] = useState("");

	function reset() {
		setName("");
		setCollection("");
		onClose();
	}

	return (
		<CreatePanel
			open={open}
			title="New database"
			icon={Leaf}
			description="MongoDB has no standalone create-database, so a first collection is required — one with nothing in it would not survive."
			dirty={name !== "" || collection !== ""}
			onClose={reset}
			onSubmit={() => createDatabase({ name, collection })}
		>
			<FormField label="Name" htmlFor="mongo-database-name">
				<Input
					id="mongo-database-name"
					value={name}
					onChange={(event) => setName(event.target.value)}
					placeholder="events"
					autoComplete="off"
					required
				/>
			</FormField>

			<FormField label="First collection" htmlFor="mongo-first-collection">
				<Input
					id="mongo-first-collection"
					value={collection}
					onChange={(event) => setCollection(event.target.value)}
					placeholder="ingested"
					autoComplete="off"
					required
				/>
			</FormField>
		</CreatePanel>
	);
}
