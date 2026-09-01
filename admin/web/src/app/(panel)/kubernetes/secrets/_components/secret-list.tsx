"use client";

import { useState } from "react";
import { KeyRound, Lock, Plus, RefreshCw, Trash2 } from "lucide-react";
import { deleteSecret } from "@/app/actions/kube";
import { Button, IconButton, InlineDeleteConfirm, Td, Th } from "@/components/ui";
import { ActionsHeader, Directory } from "../../../_components/directory";
import { useRowDelete } from "../../../_components/use-row-delete";
import { formatAge } from "@/lib/format/age";
import type { SecretSummary } from "@/lib/api/types";
import { CreateSecretPanel } from "./create-secret-panel";
import { RotateSecretPanel } from "./rotate-secret-panel";

export function SecretList({
	namespace,
	secrets,
	now,
	loadError,
}: {
	namespace: string;
	secrets: SecretSummary[];
	now: Date;
	loadError: string | null;
}) {
	const [error, setError] = useState<string | null>(loadError);
	const [creating, setCreating] = useState(false);
	const [rotating, setRotating] = useState<SecretSummary | null>(null);
	const rowDelete = useRowDelete(setError);

	const create = (
		<Button icon={Plus} onClick={() => setCreating(true)} disabled={!namespace}>
			New Secret
		</Button>
	);

	return (
		<>
			<div className="mb-4 flex justify-end">{create}</div>

			<Directory
				error={error}
				isEmpty={secrets.length === 0}
				minWidth="min-w-[720px]"
				empty={{
					icon: KeyRound,
					title: "No Secrets",
					description: "Nothing has stored a credential in this namespace yet.",
					action: create,
				}}
				columns={
					<>
						<Th>Name</Th>
						<Th>Keys</Th>
						{/* Type earns a column because it is what decides whether a row
						    has buttons, and a reason with nowhere to sit ends up in the
						    actions cell — which is as narrow as an icon. */}
						<Th>Type</Th>
						<Th className="w-px whitespace-nowrap text-right">Age</Th>
						<ActionsHeader />
					</>
				}
				rows={secrets.map((secret) => (
					<tr key={secret.name}>
						<Td className="font-medium">{secret.name}</Td>
						<Td className="text-muted-foreground">
							<Keys secret={secret} />
						</Td>
						<Td className="text-muted-foreground">
							<Type secret={secret} />
						</Td>
						<Td className="w-px whitespace-nowrap text-right text-muted-foreground">
							{formatAge(secret.createdAt, now)}
						</Td>
						<Td className="text-right">
							<SecretActions
								secret={secret}
								namespace={namespace}
								rowDelete={rowDelete}
								onRotate={() => setRotating(secret)}
							/>
						</Td>
					</tr>
				))}
			/>

			<CreateSecretPanel
				open={creating}
				namespace={namespace}
				onClose={() => setCreating(false)}
			/>
			<RotateSecretPanel
				// Keyed by name so the panel re-initialises per row rather than
				// carrying the previous Secret's keys into the next one.
				key={rotating?.name}
				secret={rotating}
				namespace={namespace}
				onClose={() => setRotating(null)}
			/>
		</>
	);
}

/**
 * The key names, which are the whole of what this panel knows about a Secret's
 * contents — and enough to point a chart's `existingSecret` at one.
 *
 * A Secret with no keys renders an em dash rather than nothing: an empty cell in
 * the middle of a table reads as missing data instead of as a deliberate no.
 */
function Keys({ secret }: { secret: SecretSummary }) {
	if (secret.keys.length === 0) {
		return <span className="text-muted-foreground/60">&mdash;</span>;
	}

	return (
		<span className="flex flex-wrap gap-1">
			{secret.keys.map((key) => (
				<code key={key} className="rounded-chip bg-surface-sunken px-1 py-0.5 text-xs">
					{key}
				</code>
			))}
		</span>
	);
}

/**
 * The Secret's type, and — where there is one — why the panel will not touch it.
 *
 * The reason is shown rather than implied. A greyed-out trash icon invites a
 * hover to find out why and says nothing until then, which is the same call the
 * namespace list makes about protection.
 */
function Type({ secret }: { secret: SecretSummary }) {
	return (
		<span className="flex flex-col gap-0.5">
			<span className="text-xs">{secret.type || "Opaque"}</span>
			{secret.reason ? (
				<span className="inline-flex items-center gap-1.5 text-xs whitespace-nowrap">
					<Lock className="h-3.5 w-3.5 shrink-0" />
					{secret.reason}
				</span>
			) : null}
			{secret.immutable ? <span className="text-xs">immutable</span> : null}
		</span>
	);
}

/**
 * Rotate and delete, or neither.
 *
 * `removable` is the API's answer, not one worked out here, so a button cannot
 * appear where the request would come back 403. A row that has none shows its
 * reason in the Type column instead.
 *
 * Rotate is additionally withheld from an immutable Secret: Kubernetes refuses
 * every write to one, and deleting it is still legitimate — immutability is a
 * rule about changing a Secret, not about removing it.
 */
function SecretActions({
	secret,
	namespace,
	rowDelete,
	onRotate,
}: {
	secret: SecretSummary;
	namespace: string;
	rowDelete: ReturnType<typeof useRowDelete>;
	onRotate: () => void;
}) {
	if (!secret.removable) {
		return null;
	}

	if (rowDelete.confirmingId === secret.name) {
		return (
			<InlineDeleteConfirm
				label="Delete this Secret? Anything reading it will break at its next restart."
				confirmLabel="Delete"
				busy={rowDelete.isDeleting(secret.name)}
				onConfirm={() =>
					rowDelete.confirm(secret.name, () => deleteSecret(namespace, secret.name))
				}
				onCancel={rowDelete.cancel}
			/>
		);
	}

	return (
		<div className="flex items-center justify-end gap-1">
			{secret.immutable ? null : (
				<IconButton aria-label={`Rotate ${secret.name}`} onClick={onRotate}>
					<RefreshCw className="h-4 w-4" />
				</IconButton>
			)}
			<IconButton
				variant="danger"
				aria-label={`Delete ${secret.name}`}
				onClick={() => rowDelete.ask(secret.name)}
			>
				<Trash2 className="h-4 w-4" />
			</IconButton>
		</div>
	);
}
