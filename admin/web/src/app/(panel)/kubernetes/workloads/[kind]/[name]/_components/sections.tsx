import { AlertTriangle, Container, HardDrive, Network } from "lucide-react";
import { SectionCard } from "@/components/ui/card";
import { Td, Th } from "@/components/ui/table";
import { Directory } from "../../../../../_components/directory";
import { formatAge } from "@/lib/format/age";
import { formatBytes } from "@/lib/format/bytes";
import type { WorkloadDetail } from "@/lib/api/types";

/**
 * The four tables under a workload's summary.
 *
 * Split out of page.tsx only for its length. Each is a Server Component and each
 * passes `empty.icon` — a function — so none of them may become "use client";
 * see the note in _components/directory.tsx.
 *
 * They pass `error={null}` throughout: the whole detail is one call, so a failure
 * is reported once at the top of the page rather than four times over.
 */

export function Pods({ detail }: { detail: WorkloadDetail }) {
	const now = new Date();
	return (
		<SectionCard title="Pods" icon={Container} padding="sm">
			<Directory
				error={null}
				isEmpty={detail.pods.length === 0}
				minWidth="min-w-[640px]"
				empty={{ icon: Container, title: "No pods", description: "Nothing is running for this workload." }}
				columns={
					<>
						<Th>Name</Th>
						<Th>Status</Th>
						<Th className="text-right">Ready</Th>
						<Th className="text-right">Restarts</Th>
						<Th>Node</Th>
						<Th className="text-right">Age</Th>
					</>
				}
				rows={detail.pods.map((pod) => (
					<tr key={pod.name}>
						<Td className="font-medium">{pod.name}</Td>
						<Td className="text-muted-foreground">{pod.status}</Td>
						<Td className="text-right text-muted-foreground">{pod.ready}</Td>
						<Td
							className={`text-right ${pod.restarts > 0 ? "text-warning-fg" : "text-muted-foreground"}`}
						>
							{pod.restarts}
						</Td>
						<Td className="text-muted-foreground">{pod.node || "—"}</Td>
						<Td className="text-right text-muted-foreground">{formatAge(pod.createdAt, now)}</Td>
					</tr>
				))}
			/>
		</SectionCard>
	);
}

export function Networking({ detail }: { detail: WorkloadDetail }) {
	return (
		<SectionCard title="Services" icon={Network} padding="sm">
			<Directory
				error={null}
				isEmpty={detail.services.length === 0}
				minWidth="min-w-[560px]"
				empty={{
					icon: Network,
					title: "No services",
					description: "Nothing routes to this workload's pods.",
				}}
				columns={
					<>
						<Th>Name</Th>
						<Th>Type</Th>
						<Th>Cluster IP</Th>
						<Th>Ports</Th>
					</>
				}
				rows={detail.services.map((service) => (
					<tr key={service.name}>
						<Td className="font-medium">{service.name}</Td>
						<Td className="text-muted-foreground">{service.type}</Td>
						<Td className="font-mono text-xs text-muted-foreground">{service.clusterIP || "—"}</Td>
						<Td className="font-mono text-xs text-muted-foreground">
							{service.ports.join(", ") || "—"}
						</Td>
					</tr>
				))}
			/>
		</SectionCard>
	);
}

export function Storage({ detail }: { detail: WorkloadDetail }) {
	return (
		<SectionCard title="Volume claims" icon={HardDrive} padding="sm">
			<Directory
				error={null}
				isEmpty={detail.claims.length === 0}
				minWidth="min-w-[560px]"
				empty={{
					icon: HardDrive,
					title: "No volume claims",
					description: "This workload mounts no persistent storage.",
				}}
				columns={
					<>
						<Th>Name</Th>
						<Th>Status</Th>
						<Th className="text-right">Size</Th>
						<Th>Class</Th>
					</>
				}
				rows={detail.claims.map((claim) => (
					<tr key={claim.name}>
						<Td className="font-medium">{claim.name}</Td>
						<Td className={claim.status === "Bound" ? "text-muted-foreground" : "text-warning-fg"}>
							{claim.status}
						</Td>
						<Td className="text-right font-mono text-xs text-muted-foreground">
							{claim.capacityBytes > 0
								? formatBytes(claim.capacityBytes)
								: `${formatBytes(claim.requestedBytes)} requested`}
						</Td>
						<Td className="text-muted-foreground">{claim.storageClass || "—"}</Td>
					</tr>
				))}
			/>
		</SectionCard>
	);
}

export function Events({ detail }: { detail: WorkloadDetail }) {
	const now = new Date();
	return (
		<SectionCard title="Events" icon={AlertTriangle} padding="sm">
			<Directory
				error={null}
				isEmpty={detail.events.length === 0}
				minWidth="min-w-[640px]"
				empty={{
					icon: AlertTriangle,
					title: "No events",
					description: "Nothing has happened to this workload recently.",
				}}
				columns={
					<>
						<Th>Type</Th>
						<Th>Reason</Th>
						<Th>Object</Th>
						<Th>Message</Th>
						<Th className="text-right">Last seen</Th>
					</>
				}
				rows={detail.events.map((event, index) => (
					<tr key={`${event.object}-${event.reason}-${index}`}>
						<Td className={event.type === "Warning" ? "text-warning-fg" : "text-muted-foreground"}>
							{event.type}
						</Td>
						<Td>{event.reason}</Td>
						<Td className="text-muted-foreground">{event.object}</Td>
						<Td className="text-muted-foreground">{event.message}</Td>
						<Td className="text-right text-muted-foreground">{formatAge(event.lastSeen, now)}</Td>
					</tr>
				))}
			/>
		</SectionCard>
	);
}
