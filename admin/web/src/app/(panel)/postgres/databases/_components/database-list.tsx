"use client";

import { useState } from "react";
import { Database, Pencil, Plus, Trash2 } from "lucide-react";
import { createDatabase, dropDatabase } from "@/app/actions/postgres";
import { Button, IconButton, InlineDeleteConfirm, Td, Th } from "@/components/ui";
import { ActionsHeader, Directory } from "../../../_components/directory";
import { useRowDelete } from "../../../_components/use-row-delete";
import { formatBytes } from "@/lib/format/bytes";
import type { PostgresDatabase } from "@/lib/api/types";
import { CreateDatabasePanel } from "./create-database-panel";
import { EditDatabasePanel } from "./edit-database-panel";

export function DatabaseList({
	databases,
	owners,
	loadError,
}: {
	databases: PostgresDatabase[];
	owners: string[];
	loadError: string | null;
}) {
	const [error, setError] = useState<string | null>(loadError);
	const [creating, setCreating] = useState(false);
	const [editing, setEditing] = useState<PostgresDatabase | null>(null);
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
				minWidth="min-w-[640px]"
				empty={{
					icon: Database,
					title: "No databases",
					description: "Create one to start using this server.",
					action: create,
				}}
				columns={
					<>
						<Th>Name</Th>
						<Th>Owner</Th>
						<Th>Encoding</Th>
						<Th className="text-right">Size</Th>
						<ActionsHeader />
					</>
				}
				rows={databases.map((database) => (
					<tr key={database.name}>
						<Td className="font-medium">{database.name}</Td>
						<Td className="text-muted-foreground">{database.owner}</Td>
						<Td className="text-muted-foreground">{database.encoding}</Td>
						{/* A size of zero means the API could not read it — the connecting
						    role has no CONNECT privilege — not that the database is empty. */}
						<Td className="text-right text-muted-foreground">
							{database.sizeBytes > 0 ? formatBytes(database.sizeBytes) : "—"}
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
								/* Pencil then Trash2, per the UX guidelines' row-action order. */
								<div className="flex items-center justify-end gap-1">
									<IconButton
										aria-label={`Edit ${database.name}`}
										onClick={() => setEditing(database)}
									>
										<Pencil className="h-4 w-4" />
									</IconButton>
									<IconButton
										variant="danger"
										aria-label={`Drop ${database.name}`}
										onClick={() => rowDelete.ask(database.name)}
									>
										<Trash2 className="h-4 w-4" />
									</IconButton>
								</div>
							)}
						</Td>
					</tr>
				))}
			/>

			<CreateDatabasePanel
				open={creating}
				owners={owners}
				onClose={() => setCreating(false)}
				onSubmit={createDatabase}
			/>
			{/* Keyed by database, so opening a different one starts from its own
			    owner rather than from whatever was last edited. */}
			<EditDatabasePanel
				key={editing?.name}
				database={editing}
				owners={owners}
				onClose={() => setEditing(null)}
			/>
		</>
	);
}
