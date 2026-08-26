"use client";

import { useState } from "react";
import { Leaf, Plus, Trash2 } from "lucide-react";
import { dropDatabase } from "@/app/actions/mongo";
import { Button, IconButton, InlineDeleteConfirm, Td, Th } from "@/components/ui";
import { ActionsHeader, Directory } from "../../../_components/directory";
import { useRowDelete } from "../../../_components/use-row-delete";
import { formatBytes } from "@/lib/format/bytes";
import type { MongoDatabase } from "@/lib/api/types";
import { CreateMongoDatabasePanel } from "./create-mongo-database-panel";

export function MongoDatabaseList({
	databases,
	loadError,
}: {
	databases: MongoDatabase[];
	loadError: string | null;
}) {
	const [error, setError] = useState<string | null>(loadError);
	const [creating, setCreating] = useState(false);
	const rowDelete = useRowDelete(setError);

	const create = (
		<Button icon={Plus} onClick={() => setCreating(true)}>
			New database
		</Button>
	);

	return (
		<>
			<div className="mb-4 flex justify-end">{create}</div>

			<Directory
				error={error}
				isEmpty={databases.length === 0}
				minWidth="min-w-[560px]"
				empty={{
					icon: Leaf,
					title: "No databases",
					description: "Create one to start using this server.",
					action: create,
				}}
				columns={
					<>
						<Th>Name</Th>
						<Th className="text-right">Size</Th>
						<Th>State</Th>
						<ActionsHeader />
					</>
				}
				rows={databases.map((database) => (
					<tr key={database.name}>
						<Td className="font-medium">{database.name}</Td>
						<Td className="text-right text-muted-foreground">
							{database.empty ? "—" : formatBytes(database.sizeBytes)}
						</Td>
						{/* MongoDB has no empty databases: one exists only while it holds a
						    collection. This marks the ones still waiting for a first write,
						    which the server itself would not list at all. */}
						<Td className="text-muted-foreground">
							{database.empty ? "Awaiting first write" : "Active"}
						</Td>
						<Td className="text-right">
							{rowDelete.confirmingId === database.name ? (
								<InlineDeleteConfirm
									label="Drop database?"
									confirmLabel="Drop"
									busy={rowDelete.isDeleting(database.name)}
									onConfirm={() =>
										rowDelete.confirm(database.name, () => dropDatabase(database.name))
									}
									onCancel={rowDelete.cancel}
								/>
							) : (
								<IconButton
									variant="danger"
									aria-label={`Drop ${database.name}`}
									onClick={() => rowDelete.ask(database.name)}
								>
									<Trash2 className="h-4 w-4" />
								</IconButton>
							)}
						</Td>
					</tr>
				))}
			/>

			<CreateMongoDatabasePanel open={creating} onClose={() => setCreating(false)} />
		</>
	);
}
