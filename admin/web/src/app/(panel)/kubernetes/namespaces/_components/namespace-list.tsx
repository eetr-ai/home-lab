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
						{/* Protection is a property of the namespace, not something you
						    can do to it, so it gets a column of its own. It used to sit
						    in the actions cell, which is deliberately as narrow as an
						    icon — a sentence in there wrapped to three ragged lines and
						    dragged the row height with it. */}
						<Th>Protection</Th>
						<Th className="w-px whitespace-nowrap text-right">Age</Th>
						<ActionsHeader />
					</>
				}
				rows={namespaces.map((namespace) => (
					<tr key={namespace.name}>
						<Td className="font-medium">{namespace.name}</Td>
						<Td className="text-muted-foreground">{namespace.status}</Td>
						<Td className="text-muted-foreground">
							<Protection namespace={namespace} />
						</Td>
						<Td className="w-px whitespace-nowrap text-right text-muted-foreground">
							{formatAge(namespace.createdAt, now)}
						</Td>
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
 * Why a namespace cannot be deleted, in the words the API used.
 *
 * A greyed-out trash icon invites a hover to find out why and says nothing until
 * then, so the reason is shown rather than implied. It reads as a sentence
 * completing the namespace name — "kube-system is a Kubernetes system namespace"
 * — which is why it is lower case and not a badge.
 *
 * Unprotected rows render an em dash rather than nothing: an empty cell in the
 * middle of a table reads as missing data instead of as a deliberate no.
 */
function Protection({ namespace }: { namespace: Namespace }) {
	if (!namespace.protected) {
		return <span className="text-muted-foreground/60">&mdash;</span>;
	}

	return (
		<span className="inline-flex items-center gap-1.5 whitespace-nowrap text-xs">
			<Lock className="h-3.5 w-3.5 shrink-0" />
			{namespace.protectedReason ?? "protected"}
		</span>
	);
}

/**
 * The delete action, and nothing else.
 *
 * A protected namespace has none: there is nothing to disable, because the API
 * would refuse it anyway, and the reason lives in its own column.
 */
function NamespaceActions({
	namespace,
	rowDelete,
}: {
	namespace: Namespace;
	rowDelete: ReturnType<typeof useRowDelete>;
}) {
	if (namespace.protected) {
		return null;
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
