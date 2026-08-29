import Link from "next/link";
import { Package } from "lucide-react";
import { listReleases } from "@/app/actions/helm";
import { Directory } from "../../_components/directory";
import { StatusBadge } from "../_components/status-badge";
import { Td, Th } from "@/components/ui/table";
import { formatAge } from "@/lib/format/age";

export const dynamic = "force-dynamic";

/**
 * Every release in every managed namespace.
 *
 * A Server Component with no client half: there is nothing to interact with here.
 * Installing happens from the catalog tab, and everything that can be done to a
 * release is done on the release's own page — a list where each row has four
 * actions is a list nobody can scan.
 */
export default async function HelmReleasesPage() {
	const releases = await listReleases();
	const now = new Date();

	return (
		<Directory
			error={releases.ok ? null : releases.error}
			isEmpty={releases.ok && releases.data.length === 0}
			minWidth="min-w-[720px]"
			empty={{
				icon: Package,
				title: "No releases",
				description: "Nothing is installed in the namespaces this lab manages.",
			}}
			columns={
				<>
					<Th>Release</Th>
					<Th>Namespace</Th>
					<Th>Chart</Th>
					<Th>Status</Th>
					<Th className="text-right">Updated</Th>
				</>
			}
			rows={(releases.ok ? releases.data : []).map((release) => (
				<tr key={`${release.namespace}/${release.name}`}>
					<Td className="font-medium">
						<Link
							href={`/helm/releases/${encodeURIComponent(release.namespace)}/${encodeURIComponent(release.name)}`}
							className="rounded-control outline-none ring-brand hover:underline focus-visible:ring-2"
						>
							{release.name}
						</Link>
					</Td>
					<Td className="text-muted-foreground">{release.namespace}</Td>
					<Td className="text-muted-foreground">
						{release.chart} {release.chartVersion}
					</Td>
					<Td>
						<StatusBadge status={release.status} />
					</Td>
					<Td className="text-right text-muted-foreground">
						{formatAge(release.updatedAt, now)}
					</Td>
				</tr>
			))}
		/>
	);
}
