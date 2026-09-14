import { cookies } from "next/headers";
import { ALL_NAMESPACES, isNamespaceName, readScope, scopeCookie, type NamespaceScope } from "@/lib/kube/scope";

/**
 * The namespace scope a cluster page should render, as a Server Component sees it.
 *
 * Normally the cookie the picker writes, which is what makes one choice apply to
 * every page instead of each page resolving one of its own.
 *
 * A `?namespace=` in the address wins over it. That parameter is no longer where
 * the scope lives, but it is still how something arrives at a page already
 * knowing which namespace it means: a link out of a workload's detail page, a
 * bookmark, and the agent, whose route catalogue names it. The picker adopts
 * whatever the page resolved, so following such a link also moves the shared
 * scope — arriving somewhere and then finding the picker disagreeing with the
 * rows under it is the confusion this whole change is about.
 *
 * It does not check that the namespace still exists — that needs the namespace
 * list, and a page fetching one just to validate a cookie would be a second round
 * trip on every render. The provider does that check on the client, where it can
 * also fix it; see NamespaceScopeProvider.
 */
export async function readNamespaceScope(requested?: string): Promise<NamespaceScope> {
	if (requested === ALL_NAMESPACES) return ALL_NAMESPACES;
	if (requested !== undefined && isNamespaceName(requested)) return requested;

	const jar = await cookies();
	return readScope(jar.get(scopeCookie)?.value);
}
