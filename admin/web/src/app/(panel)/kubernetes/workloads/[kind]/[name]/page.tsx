import { Boxes } from "lucide-react";
import { notFound } from "next/navigation";
import { readWorkload } from "@/app/actions/kube";
import { BackLink } from "../../../../_components/back-link";
import { Banner } from "@/components/ui/banner";
import { SectionCard } from "@/components/ui/card";
import { formatAge } from "@/lib/format/age";
import { Events, Networking, Pods, Storage } from "./_components/sections";
import { WorkloadControls } from "./_components/workload-controls";
import type { WorkloadDetail } from "@/lib/api/types";

export const dynamic = "force-dynamic";

/** The kinds the API reports, in the spelling it uses. */
const KINDS = new Set(["Deployment", "StatefulSet", "DaemonSet"]);

/**
 * One workload and everything around it.
 *
 * The namespace is a query parameter rather than another path segment, which
 * keeps it consistent with the list pages this is reached from — they are all
 * `?namespace=`, and the scope survives the navigation.
 */
export default async function WorkloadPage({
	params,
	searchParams,
}: {
	params: Promise<{ kind: string; name: string }>;
	searchParams: Promise<{ namespace?: string }>;
}) {
	const { kind, name } = await params;
	const { namespace } = await searchParams;

	// Checked here so a mistyped URL is a 404 rather than a banner explaining that
	// the API rejected a kind the operator never chose.
	if (!KINDS.has(kind) || !namespace) notFound();

	const detail = await readWorkload(kind, namespace, name);
	if (!detail.ok) {
		return (
			<>
				<BackLink href={`/kubernetes/workloads?namespace=${encodeURIComponent(namespace)}`} label="All workloads" />
				<Banner variant="error" message={detail.error} />
			</>
		);
	}

	return (
		<div className="flex flex-col gap-6">
			<BackLink href={`/kubernetes/workloads?namespace=${encodeURIComponent(namespace)}`} label="All workloads" />
			<Summary detail={detail.data} namespace={namespace} kind={kind} name={name} />
			<Pods detail={detail.data} />
			<Networking detail={detail.data} />
			<Storage detail={detail.data} />
			<Events detail={detail.data} />
		</div>
	);
}

function Summary({
	detail,
	namespace,
	kind,
	name,
}: {
	detail: WorkloadDetail;
	namespace: string;
	kind: string;
	name: string;
}) {
	const { workload } = detail;
	const degraded = workload.ready < workload.desired;

	return (
		<SectionCard title={`${kind} ${name}`} icon={Boxes}>
			<dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
				<dt className="text-muted-foreground">Ready</dt>
				<dd className={degraded ? "text-warning-fg" : ""}>
					{workload.ready} / {workload.desired}
					{/* Updated and available are what tell a rollout that has stalled
					    from one that is merely still going. */}
					<span className="ml-2 text-muted-foreground">
						{detail.updated} updated, {detail.available} available
					</span>
				</dd>
				<dt className="text-muted-foreground">Images</dt>
				<dd className="break-all font-mono text-xs">{workload.images.join(", ") || "—"}</dd>
				<dt className="text-muted-foreground">Age</dt>
				<dd>{formatAge(workload.createdAt, new Date())}</dd>
			</dl>

			{detail.conditions.length > 0 ? (
				<ul className="mt-4 space-y-1 text-sm">
					{detail.conditions.map((condition) => (
						<li key={condition.type} className="flex flex-wrap gap-2">
							<span className={condition.status === "True" ? "" : "text-warning-fg"}>
								{condition.type}: {condition.status}
							</span>
							{condition.message ? (
								<span className="text-muted-foreground">{condition.message}</span>
							) : null}
						</li>
					))}
				</ul>
			) : null}

			<div className="mt-4 border-t border-border pt-4">
				<WorkloadControls
					kind={kind}
					namespace={namespace}
					name={name}
					scale={detail.scale}
				/>
			</div>
		</SectionCard>
	);
}
