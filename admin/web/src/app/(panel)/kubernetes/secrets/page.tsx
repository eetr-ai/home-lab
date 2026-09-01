import { listSecrets } from "@/app/actions/kube";
import { ScopePicker } from "../../_components/scope-picker";
import { resolveNamespace } from "../_components/namespace-scope";
import { SecretList } from "./_components/secret-list";

export const dynamic = "force-dynamic";

/**
 * The Secrets in one namespace.
 *
 * Namespace-scoped like the pods and events tabs, and for the same reason: the
 * grant that lets the panel read a Secret is bound per namespace, so "every
 * Secret on the cluster" is not a question this API can answer.
 *
 * Nothing here is a value. The listing carries names, types and key names, and
 * there is no route that would return more — see internal/kube/secrets.go.
 */
export default async function SecretsPage({
	searchParams,
}: {
	searchParams: Promise<{ namespace?: string }>;
}) {
	const { namespace: requested } = await searchParams;
	const { namespaces, selected, error } = await resolveNamespace(requested);
	const secrets = selected ? await listSecrets(selected) : null;

	return (
		<>
			<ScopePicker label="Namespace" param="namespace" options={namespaces} selected={selected} />
			<SecretList
				namespace={selected}
				secrets={secrets?.ok ? secrets.data : []}
				// Ages against one instant taken on the server, the same as every
				// other cluster list. Taking it in the client component would give
				// the server and the browser two different answers for one row.
				now={new Date()}
				loadError={error ?? (secrets && !secrets.ok ? secrets.error : null)}
			/>
		</>
	);
}
