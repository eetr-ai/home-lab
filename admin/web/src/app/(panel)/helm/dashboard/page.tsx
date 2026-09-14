import { listNamespaceReleases, listReleases } from "@/app/actions/helm";
import { NamespacePicker } from "../../_components/namespace-picker";
import { readNamespaceScope } from "../../_components/namespace-scope-server";
import { isAllNamespaces } from "@/lib/kube/scope";
import { ReleaseTable } from "./_components/release-table";

export const dynamic = "force-dynamic";

/**
 * Everything Helm has, which is not the same as everything this lab declared.
 *
 * The first tab on purpose: it answers "what is actually running", including the
 * releases nobody declared here — the platform chart, the panel itself, anything
 * installed by hand. Deployments answer a different question, and putting this
 * first means the section opens on the cluster rather than on a record of it.
 *
 * With a namespace chosen, the narrower per-namespace read is used rather than
 * filtering a cluster-wide one. Reading a release means reading Secrets, so
 * asking for one namespace instead of all of them is worth the extra branch.
 *
 * The namespace is the panel's shared scope rather than this page's own, so
 * arriving here from the cluster section keeps looking at the same namespace.
 */
export default async function HelmDashboardPage({
	searchParams,
}: {
	searchParams: Promise<{ namespace?: string }>;
}) {
	const { namespace } = await searchParams;
	const scope = await readNamespaceScope(namespace);
	const everywhere = isAllNamespaces(scope);
	const releases = everywhere ? await listReleases() : await listNamespaceReleases(scope);

	return (
		<>
			<NamespacePicker scope={scope} />
			<ReleaseTable
				releases={releases.ok ? releases.data : []}
				loadError={releases.ok ? null : releases.error}
				scoped={!everywhere}
				now={new Date()}
			/>
		</>
	);
}
