/**
 * Which pages the namespace scope applies to, and how.
 *
 * - `optional`: a list that can answer for one namespace or the whole cluster.
 * - `required`: a list that can only answer for one namespace. Secrets is the
 *   one — the panel's grant for them is bound per namespace.
 * - `none`: everything else. Cluster-scoped listings (Nodes, Storage), the
 *   namespace management page, and detail pages, which are about one object in
 *   one namespace — a switcher above those would look like it applied.
 *
 * Exact matches on purpose: a detail page lives under its list's path, and
 * inheriting the list's answer is exactly the mistake described above.
 */
export type NamespaceScopeUse = "none" | "optional" | "required";

const scopedRoutes: Record<string, NamespaceScopeUse> = {
	"/kubernetes/workloads": "optional",
	"/kubernetes/pods": "optional",
	"/kubernetes/jobs": "optional",
	"/kubernetes/events": "optional",
	"/kubernetes/secrets": "required",
	"/helm/dashboard": "optional",
	"/helm/deployments": "optional",
};

/** How the page at `pathname` uses the namespace scope. */
export function namespaceScopeFor(pathname: string): NamespaceScopeUse {
	const path = pathname.length > 1 ? pathname.replace(/\/+$/, "") : pathname;
	return scopedRoutes[path] ?? "none";
}
