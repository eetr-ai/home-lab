import { KeyRound } from "lucide-react";
import { listSecrets } from "@/app/actions/kube";
import { EmptyState } from "@/components/ui/empty-state";
import { NamespacePicker } from "../../_components/namespace-picker";
import { readNamespaceScope } from "../../_components/namespace-scope-server";
import { isAllNamespaces } from "@/lib/kube/scope";
import { SecretList } from "./_components/secret-list";

export const dynamic = "force-dynamic";

/**
 * The Secrets in one namespace.
 *
 * The one cluster page that will not answer for the whole cluster, and not by
 * omission: the grant that lets the panel read a Secret is a RoleBinding created
 * per namespace by enrolment, where pods and workloads are covered by a
 * ClusterRole. "Every Secret on the cluster" is not a question this API can
 * answer, so the page asks for a namespace rather than showing a partial list
 * and calling it the cluster.
 *
 * Nothing here is a value. The listing carries names, types and key names, and
 * there is no route that would return more — see internal/kube/secrets.go.
 */
export default async function SecretsPage({
	searchParams,
}: {
	searchParams: Promise<{ namespace?: string }>;
}) {
	const { namespace } = await searchParams;
	const scope = await readNamespaceScope(namespace);

	if (isAllNamespaces(scope)) {
		return (
			<>
				<NamespacePicker scope={scope} requireOne />
				<EmptyState
					icon={KeyRound}
					title="Choose a namespace"
					description="Secrets are read through a grant the panel holds one namespace at a time, so there is no cluster-wide list of them to show."
				/>
			</>
		);
	}

	const secrets = await listSecrets(scope);

	return (
		<>
			<NamespacePicker scope={scope} requireOne />
			<SecretList
				namespace={scope}
				secrets={secrets.ok ? secrets.data : []}
				// Ages against one instant taken on the server, the same as every
				// other cluster list. Taking it in the client component would give
				// the server and the browser two different answers for one row.
				now={new Date()}
				loadError={secrets.ok ? null : secrets.error}
			/>
		</>
	);
}
