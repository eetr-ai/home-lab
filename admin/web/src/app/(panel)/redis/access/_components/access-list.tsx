"use client";

import { useState, useTransition } from "react";
import { Network, ShieldOff, ShieldPlus } from "lucide-react";
import { setRedisAccess } from "@/app/actions/redis";
import { IconButton, InlineDeleteConfirm, Spinner, Td, Th } from "@/components/ui";
import { hasRedisAccess } from "@/lib/redis/access";
import type { Namespace } from "@/lib/api/types";
import { ActionsHeader, Directory } from "../../../_components/directory";
import { useRowDelete } from "../../../_components/use-row-delete";

/**
 * Every namespace and whether it may reach Redis.
 *
 * Granting is one click: it opens a door that still needs a password. Revoking
 * confirms inline, because it is the one that breaks something — the namespace's
 * pods are refused the next time they connect.
 *
 * Protected namespaces are shown and not offered. platform-system is Redis's own,
 * and the panel's namespace losing the label would take the panel's sign-in down;
 * the API refuses both, and a button that is always refused is worse than none.
 */
export function AccessList({ namespaces, loadError }: { namespaces: Namespace[]; loadError: string | null }) {
	const [error, setError] = useState<string | null>(loadError);
	const [granting, setGranting] = useState<string | null>(null);
	const [pending, startTransition] = useTransition();
	const revoke = useRowDelete(setError);

	function grant(name: string) {
		setError(null);
		setGranting(name);
		startTransition(async () => {
			const result = await setRedisAccess(name, true);
			setGranting(null);
			if (!result.ok) setError(result.error);
		});
	}

	return (
		<>
			<p className="mb-4 max-w-2xl text-sm text-muted-foreground">
				Pods in an allowed namespace can open a connection; they still need a user and its password to do
				anything. Revoking does not cut connections already open.
			</p>

			<Directory
				error={error}
				isEmpty={namespaces.length === 0}
				minWidth="min-w-[560px]"
				empty={{ icon: Network, title: "No namespaces", description: "The cluster's namespaces could not be listed." }}
				columns={
					<>
						<Th>Namespace</Th>
						<Th>Redis</Th>
						<ActionsHeader />
					</>
				}
				rows={namespaces.map((namespace) => {
					const allowed = hasRedisAccess(namespace);
					return (
						<tr key={namespace.name}>
							<Td className="font-medium">{namespace.name}</Td>
							<Td className={allowed ? "" : "text-muted-foreground"}>
								{allowed ? "Allowed" : "Not allowed"}
								{namespace.protected ? (
									<span className="ml-2 text-xs text-muted-foreground">
										protected{namespace.protectedReason ? `: ${namespace.protectedReason}` : ""}
									</span>
								) : null}
							</Td>
							<Td className="text-right">
								{namespace.protected ? null : revoke.confirmingId === namespace.name ? (
									<InlineDeleteConfirm
										label="Revoke access?"
										confirmLabel="Revoke"
										busy={revoke.isDeleting(namespace.name)}
										onConfirm={() =>
											revoke.confirm(namespace.name, async () => {
												const result = await setRedisAccess(namespace.name, false);
												return result.ok ? { ok: true, data: undefined } : result;
											})
										}
										onCancel={revoke.cancel}
									/>
								) : allowed ? (
									<IconButton
										variant="danger"
										aria-label={`Revoke Redis access for ${namespace.name}`}
										onClick={() => revoke.ask(namespace.name)}
									>
										<ShieldOff className="h-4 w-4" />
									</IconButton>
								) : (
									<IconButton
										aria-label={`Allow ${namespace.name} to reach Redis`}
										disabled={pending}
										onClick={() => grant(namespace.name)}
									>
										{granting === namespace.name ? <Spinner /> : <ShieldPlus className="h-4 w-4" />}
									</IconButton>
								)}
							</Td>
						</tr>
					);
				})}
			/>
		</>
	);
}
