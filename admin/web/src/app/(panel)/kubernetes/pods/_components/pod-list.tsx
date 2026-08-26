"use client";

import { useState } from "react";
import { Container, ScrollText } from "lucide-react";
import { IconButton, Td, Th } from "@/components/ui";
import { ActionsHeader, Directory } from "../../../_components/directory";
import { formatAge } from "@/lib/format/age";
import type { Pod } from "@/lib/api/types";
import { LogPanel } from "../../_components/log-panel";

/**
 * The pods table, with a log panel that opens under it.
 *
 * A Client Component because opening the panel is browser state and the log
 * stream is a fetch the browser holds open — a Server Component can do neither.
 * The rows are still rendered from data the page fetched on the server.
 */

/**
 * The API summarizes a pod's state into one word the way `kubectl` does —
 * "Running", "CrashLoopBackOff", "Init:1/2", "Terminating" — because the raw
 * phase says "Running" for a pod whose only container is crash-looping.
 */
const UNHEALTHY = /BackOff|Error|Failed|Unknown|Evicted|OOMKilled/;

export function PodList({
	pods,
	error,
	namespace,
}: {
	pods: Pod[];
	error: string | null;
	namespace: string;
}) {
	const [tailing, setTailing] = useState<Pod | null>(null);
	const now = new Date();

	return (
		<>
			<Directory
				error={error}
				isEmpty={error === null && pods.length === 0}
				minWidth="min-w-[820px]"
				empty={{
					icon: Container,
					title: "No pods",
					description: "Nothing is scheduled in this namespace.",
				}}
				columns={
					<>
						<Th>Name</Th>
						<Th>Status</Th>
						<Th className="text-right">Ready</Th>
						<Th className="text-right">Restarts</Th>
						<Th>Node</Th>
						<Th className="text-right">Age</Th>
						<ActionsHeader />
					</>
				}
				rows={pods.map((pod) => (
					<tr key={pod.name}>
						<Td className="font-medium">{pod.name}</Td>
						<Td className={UNHEALTHY.test(pod.status) ? "text-danger-fg" : "text-muted-foreground"}>
							{pod.status}
						</Td>
						<Td className="text-right text-muted-foreground">{pod.ready}</Td>
						{/* Restarts are only worth noticing above zero, and then they are
						    worth noticing a lot. */}
						<Td
							className={`text-right ${pod.restarts > 0 ? "text-warning-fg" : "text-muted-foreground"}`}
						>
							{pod.restarts}
						</Td>
						<Td className="text-muted-foreground">{pod.node || "—"}</Td>
						<Td className="text-right text-muted-foreground">{formatAge(pod.createdAt, now)}</Td>
						<Td className="text-right">
							<IconButton
								aria-label={`View the logs for ${pod.name}`}
								title="Logs"
								onClick={() => setTailing(pod)}
							>
								<ScrollText className="h-4 w-4" />
							</IconButton>
						</Td>
					</tr>
				))}
			/>

			{tailing ? (
				// Keyed by pod: selecting a different one is a new instance, which is
				// how the buffer and the stream reset.
				<LogPanel
					key={`${namespace}/${tailing.name}`}
					namespace={namespace}
					pod={tailing.name}
					containers={tailing.containers}
					onClose={() => setTailing(null)}
				/>
			) : null}
		</>
	);
}
