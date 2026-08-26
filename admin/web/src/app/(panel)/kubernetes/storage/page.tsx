import { HardDrive } from "lucide-react";
import { readStorage } from "@/app/actions/kube";
import { SectionCard } from "@/components/ui/card";
import { Td, Th } from "@/components/ui/table";
import { Directory } from "../../_components/directory";
import { formatAge } from "@/lib/format/age";
import { formatBytes } from "@/lib/format/bytes";

export const dynamic = "force-dynamic";

/**
 * Persistent storage, from both ends.
 *
 * Claims and volumes, because they answer different questions. The claims say
 * what workloads asked for; the volumes say what exists — including a Released
 * one still holding data that no claim points at and nothing is reclaiming,
 * which appears in neither list alone.
 *
 * Cluster-wide rather than namespace-scoped: a PersistentVolume has no namespace,
 * and splitting the two halves across different scopes would make the pairing
 * unreadable.
 */
export default async function StoragePage() {
	const storage = await readStorage();
	const claims = storage.ok ? storage.data.claims : [];
	const volumes = storage.ok ? storage.data.volumes : [];
	const error = storage.ok ? null : storage.error;
	const now = new Date();

	return (
		<div className="flex flex-col gap-6">
			<SectionCard title="Volume claims" icon={HardDrive} padding="sm">
				<Directory
					error={error}
					/* Not just an empty list: a read that failed returns one too, and
					   "nothing has asked for storage" is a claim about the cluster that a
					   failed read is not entitled to make. */
					isEmpty={storage.ok && claims.length === 0}
					minWidth="min-w-[820px]"
					empty={{
						icon: HardDrive,
						title: "No volume claims",
						description: "Nothing on the cluster has asked for persistent storage yet.",
					}}
					columns={
						<>
							<Th>Namespace</Th>
							<Th>Name</Th>
							<Th>Status</Th>
							<Th className="text-right">Size</Th>
							<Th>Class</Th>
							<Th>Access</Th>
							<Th className="text-right">Age</Th>
						</>
					}
					rows={claims.map((claim) => (
						<tr key={`${claim.namespace}/${claim.name}`}>
							<Td className="text-muted-foreground">{claim.namespace}</Td>
							<Td className="font-medium">{claim.name}</Td>
							<Td className={claim.status === "Bound" ? "text-muted-foreground" : "text-warning-fg"}>
								{claim.status}
							</Td>
							{/* The granted size, falling back to what was asked for: a pending
							    claim has only the request, and showing nothing would hide how
							    much it is waiting for. */}
							<Td className="text-right font-mono text-xs text-muted-foreground">
								{claim.capacityBytes > 0
									? formatBytes(claim.capacityBytes)
									: `${formatBytes(claim.requestedBytes)} requested`}
							</Td>
							<Td className="text-muted-foreground">{claim.storageClass || "—"}</Td>
							<Td className="text-muted-foreground">{claim.accessModes.join(", ") || "—"}</Td>
							<Td className="text-right text-muted-foreground">{formatAge(claim.createdAt, now)}</Td>
						</tr>
					))}
				/>
			</SectionCard>

			<SectionCard title="Volumes" icon={HardDrive} padding="sm">
				<Directory
					/* The banner is on the claims card above — both lists come from the
					   same call, so repeating it would report one failure twice. What
					   must not survive the failure is the empty state: "nothing has been
					   provisioned" is a claim about the cluster, and a read that failed
					   is not entitled to make it. */
					error={null}
					isEmpty={storage.ok && volumes.length === 0}
					minWidth="min-w-[820px]"
					empty={{
						icon: HardDrive,
						title: "No volumes",
						description: "Nothing has been provisioned on the cluster.",
					}}
					columns={
						<>
							<Th>Name</Th>
							<Th>Status</Th>
							<Th className="text-right">Size</Th>
							<Th>Class</Th>
							<Th>Claim</Th>
							<Th>On release</Th>
							<Th className="text-right">Age</Th>
						</>
					}
					rows={volumes.map((volume) => (
						<tr key={volume.name}>
							<Td className="font-medium">{volume.name}</Td>
							{/* Released is the one worth flagging: the claim is gone, the data
							    is not, and nothing is going to reclaim it on its own. */}
							<Td className={volume.status === "Bound" ? "text-muted-foreground" : "text-warning-fg"}>
								{volume.status}
							</Td>
							<Td className="text-right font-mono text-xs text-muted-foreground">
								{formatBytes(volume.capacityBytes)}
							</Td>
							<Td className="text-muted-foreground">{volume.storageClass || "—"}</Td>
							<Td className="text-muted-foreground">{volume.claim || "—"}</Td>
							<Td className="text-muted-foreground">{volume.reclaimPolicy}</Td>
							<Td className="text-right text-muted-foreground">{formatAge(volume.createdAt, now)}</Td>
						</tr>
					))}
				/>
			</SectionCard>
		</div>
	);
}
