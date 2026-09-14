import { listPods } from "@/app/actions/kube";
import { NamespacePicker } from "../../_components/namespace-picker";
import { readNamespaceScope } from "../../_components/namespace-scope-server";
import { isAllNamespaces } from "@/lib/kube/scope";
import { PodList } from "./_components/pod-list";

export const dynamic = "force-dynamic";

/**
 * The pods in the chosen namespace, or in every namespace.
 *
 * Cluster-wide is the default and costs one request either way — the panel's
 * grant for pods is a ClusterRole, so reading one namespace and reading all of
 * them are the same permission and the API answers both from one route each.
 */
export default async function PodsPage({
	searchParams,
}: {
	searchParams: Promise<{ namespace?: string }>;
}) {
	const { namespace } = await searchParams;
	const scope = await readNamespaceScope(namespace);
	const pods = await listPods(scope);

	return (
		<>
			<NamespacePicker scope={scope} />
			<PodList
				pods={pods.ok ? pods.data : []}
				error={pods.ok ? null : pods.error}
				showNamespace={isAllNamespaces(scope)}
			/>
		</>
	);
}
