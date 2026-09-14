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
 * choose from, and the ones the API would refuse are filtered out here rather
 * than in the form: offering a choice that comes back 403 is worse than not
 * offering it. Both halves of that answer are the API's own — `helmManaged` says
 * the panel may install there at all, `protected` says it may not write there —
 * so this filter and that refusal cannot disagree. The list's failure is reported
 * separately, because "no namespaces" and "the namespaces could not be read" are
 * different sentences.
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
				namespaces={
					namespaces.ok
						? namespaces.data.filter((one) => one.helmManaged && !one.protected)
						: []
				}
				namespacesError={namespaces.ok ? null : namespaces.error}
				showNamespace={everywhere}
				now={new Date()}
			/>
		</>
	);
}
