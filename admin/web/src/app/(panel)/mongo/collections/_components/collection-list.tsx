"use client";

import { useState, useTransition } from "react";
import { FolderOpen, Plus, Trash2 } from "lucide-react";
import { createCollection, dropCollection } from "@/app/actions/mongo";
import { Button, IconButton, InlineDeleteConfirm, Input, Td, Th } from "@/components/ui";
import { ActionsHeader, Directory } from "../../../_components/directory";
import { ScopePicker } from "../../../_components/scope-picker";
import { useRowDelete } from "../../../_components/use-row-delete";
import type { MongoCollection } from "@/lib/api/types";

/** One field, so an inline add row rather than a side panel. */
export function CollectionList({
	databases,
	selected,
	collections,
	loadError,
}: {
	databases: string[];
	selected: string;
	collections: MongoCollection[];
	loadError: string | null;
}) {
	const [error, setError] = useState<string | null>(loadError);
	const [name, setName] = useState("");
	const [pending, startTransition] = useTransition();
	const rowDelete = useRowDelete(setError);

	function add(event: React.FormEvent) {
		event.preventDefault();
		if (!name.trim()) return;
		setError(null);
		startTransition(async () => {
			const result = await createCollection(selected, { name: name.trim() });
			if (!result.ok) {
				setError(result.error);
				return;
			}
			setName("");
		});
	}

	return (
		<>
			<ScopePicker label="Database" param="database" options={databases} selected={selected} />

			{selected ? (
				<form onSubmit={add} className="mb-4 flex flex-wrap items-center gap-2">
					<Input
						aria-label="Collection name"
						value={name}
						onChange={(event) => setName(event.target.value)}
						placeholder="ingested"
						autoComplete="off"
						className="w-56"
					/>
					<Button type="submit" icon={Plus} loading={pending} disabled={!name.trim()}>
						Create
					</Button>
				</form>
			) : null}

			<Directory
				error={error}
				isEmpty={collections.length === 0}
				empty={{
					icon: FolderOpen,
					title: selected ? "No collections" : "No databases",
					description: selected
						? "Create one to start writing documents."
						: "Create a database first.",
				}}
				columns={
					<>
						<Th>Name</Th>
						<Th>Type</Th>
						<ActionsHeader />
					</>
				}
				rows={collections.map((collection) => (
					<tr key={collection.name}>
						<Td className="font-medium">{collection.name}</Td>
						<Td className="text-muted-foreground">{collection.type}</Td>
						<Td className="text-right">
							{rowDelete.confirmingId === collection.name ? (
								<InlineDeleteConfirm
									label="Drop collection?"
									confirmLabel="Drop"
									busy={rowDelete.isDeleting(collection.name)}
									onConfirm={() =>
										rowDelete.confirm(collection.name, () =>
											dropCollection(selected, collection.name),
										)
									}
									onCancel={rowDelete.cancel}
								/>
							) : (
								<IconButton
									variant="danger"
									aria-label={`Drop ${collection.name}`}
									onClick={() => rowDelete.ask(collection.name)}
								>
									<Trash2 className="h-4 w-4" />
								</IconButton>
							)}
						</Td>
					</tr>
				))}
			/>
		</>
	);
}
