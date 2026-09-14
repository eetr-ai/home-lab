import { listDeployments } from "@/app/actions/helm";
import { listNamespaces } from "@/app/actions/kube";
import { NamespacePicker } from "../../_components/namespace-picker";
import { readNamespaceScope } from "../../_components/namespace-scope-server";
import { isAllNamespaces } from "@/lib/kube/scope";
import { DeploymentList } from "./_components/deployment-list";

export const dynamic = "force-dynamic";

/**
 * The charts this lab has declared, and how each stands against the cluster.
 *
 * Scoped by the panel's shared namespace like everything else, so this tab and
 * the dashboard beside it are looking at the same part of the cluster.
 *
 * The namespace list comes along because declaring a deployment needs one to
 * choose from, and protected namespaces are filtered out here rather than in the
 * form: the API refuses them, and offering a choice that would be refused is
 * worse than not offering it. Its failure is reported separately, because "no
 * namespaces" and "the namespaces could not be read" are different sentences.
 */
export default async function HelmDeploymentsPage({
	searchParams,
}: {
	searchParams: Promise<{ namespace?: string }>;
}) {
	const { namespace } = await searchParams;
	const scope = await readNamespaceScope(namespace);
	const everywhere = isAllNamespaces(scope);
	const [deployments, namespaces] = await Promise.all([
		listDeployments(everywhere ? undefined : scope),
		listNamespaces(),
	]);

	return (
		<>
			<NamespacePicker scope={scope} />
			<DeploymentList
				deployments={deployments.ok ? deployments.data : []}
				loadError={deployments.ok ? null : deployments.error}
				namespaces={namespaces.ok ? namespaces.data.filter((one) => !one.protected) : []}
				namespacesError={namespaces.ok ? null : namespaces.error}
				showNamespace={everywhere}
				now={new Date()}
			/>
		</>
	);
}
