import { listNamespaces } from "@/app/actions/kube";

/**
 * Resolve which namespace a cluster page is showing.
 *
 * Every tab in this section needs the same thing — the namespace list for the
 * picker, and one selection resolved against it — so the three pages share this
 * rather than each getting the logic subtly differently.
 *
 * A namespace named in the URL that no longer exists falls back to the first
 * rather than erroring: the link is stale, not wrong.
 */
export async function resolveNamespace(requested: string | undefined): Promise<{
	namespaces: string[];
	selected: string;
	error: string | null;
}> {
	const result = await listNamespaces();
	if (!result.ok) return { namespaces: [], selected: "", error: result.error };

	const namespaces = result.data.map((namespace) => namespace.name);
	const selected =
		requested && namespaces.includes(requested) ? requested : (namespaces[0] ?? "");
	return { namespaces, selected, error: null };
}
