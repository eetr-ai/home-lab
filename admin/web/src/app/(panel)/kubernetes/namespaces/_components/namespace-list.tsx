"use client";

import { useState } from "react";
import { Layers, Lock, Plus, Trash2 } from "lucide-react";
import { deleteNamespace } from "@/app/actions/kube";
import { Button, IconButton, InlineDeleteConfirm, Td, Th } from "@/components/ui";
import { ActionsHeader, Directory } from "../../../_components/directory";
import { useRowDelete } from "../../../_components/use-row-delete";
import { formatAge } from "@/lib/format/age";
import type { Namespace } from "@/lib/api/types";
import { CreateNamespacePanel } from "./create-namespace-panel";

export function NamespaceList({
	namespaces,
	now,
	loadError,
}: {
	namespaces: Namespace[];
	now: Date;
	loadError: string | null;
}) {
	const [error, setError] = useState<string | null>(loadError);
	const [creating, setCreating] = useState(false);
	const rowDelete = useRowDelete(setError);

	const create = (
		<Button icon={Plus} onClick={() => setCreating(true)}>
			New namespace
		</Button>
	);

	return (
		<>
			<div className="mb-4 flex justify-end">{create}</div>

			<Directory
				error={error}
				isEmpty={namespaces.length === 0}
				minWidth="min-w-[640px]"
				empty={{
					icon: Layers,
					title: "No namespaces",
					description: "Nothing is running on this cluster yet.",
					action: create,
				}}
				columns={
					<>
						<Th>Name</Th>
						<Th>Status</Th>
						<Th className="text-right">Age</Th>
						<ActionsHeader />
					</>
				}
				rows={namespaces.map((namespace) => (
					<tr key={namespace.name}>
						<Td className="font-medium">{namespace.name}</Td>
						<Td className="text-muted-foreground">{namespace.status}</Td>
						<Td className="text-right text-muted-foreground">{formatAge(namespace.createdAt, now)}</Td>
						<Td className="text-right">
							<NamespaceActions namespace={namespace} rowDelete={rowDelete} />
						</Td>
					</tr>
				))}
			/>

			<CreateNamespacePanel open={creating} onClose={() => setCreating(false)} />
		</>
	);
}

/**
 * A protected namespace gets a reason rather than a disabled button.
 *
 * A greyed-out trash icon invites a hover to find out why and says nothing until
 * then; the reason is the useful part, so it is what occupies the cell. There is
 * no delete to disable, because the API would refuse it anyway.
 */
function NamespaceActions({
	namespace,
	rowDelete,
}: {
	namespace: Namespace;
	rowDelete: ReturnType<typeof useRowDelete>;
}) {
	if (namespace.protected) {
		return (
			<span
				className="inline-flex items-center gap-1.5 text-xs text-muted-foreground"
				title={namespace.protectedReason}
			>
				<Lock className="h-3.5 w-3.5" />
				{namespace.protectedReason ?? "Protected"}
			</span>
		);
	}

	if (rowDelete.confirmingId === namespace.name) {
		return (
			<InlineDeleteConfirm
				label="Delete namespace and everything in it?"
				confirmLabel="Delete"
				busy={rowDelete.isDeleting(namespace.name)}
				onConfirm={() =>
					// force: the API refuses a namespace that still runs workloads
					// without it, and confirming here is the operator saying so.
					// It is not a way past protection — this row is not protected.
					rowDelete.confirm(namespace.name, () => deleteNamespace(namespace.name, true))
				}
				onCancel={rowDelete.cancel}
			/>
		);
	}

	return (
		<div className="flex items-center justify-end gap-1">
			<IconButton
				variant="danger"
				aria-label={`Delete ${namespace.name}`}
				onClick={() => rowDelete.ask(namespace.name)}
			>
				<Trash2 className="h-4 w-4" />
			</IconButton>
		</div>
	);
}
