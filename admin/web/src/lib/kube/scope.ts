/**
 * The namespace the panel is currently looking at.
 *
 * One value for the whole panel rather than one per page. Every cluster page
 * used to resolve its own from a query parameter and fall back to the first
 * namespace alphabetically when there was none — so moving between tabs silently
 * re-scoped you to whatever sorts first, and the only sign of it was a dropdown
 * that had been there all along. A Secret created in the wrong namespace is what
 * that costs.
 */

/**
 * The scope that means the whole cluster.
 *
 * An asterisk rather than the empty string the Kubernetes API uses: this value
 * travels through a cookie, where absent and empty are hard to tell apart, and it
 * is rendered in a `select` whose empty option means "nothing chosen". It also
 * cannot collide with a real namespace — Kubernetes names are DNS-1123 labels,
 * which have no asterisk in them.
 */
export const ALL_NAMESPACES = "*";

/**
 * A namespace name, or ALL_NAMESPACES.
 *
 * Deliberately not `string | typeof ALL_NAMESPACES`, which collapses to `string`
 * anyway. The alias is here so a signature can say which of the two kinds of
 * string it wants.
 */
export type NamespaceScope = string;

/** Where the scope is kept between requests, so a server render can read it. */
export const scopeCookie = "namespace-scope";

/**
 * How long a remembered scope outlives the tab that chose it: a week, which is
 * long enough to come back to the panel on Monday still looking at what you were
 * looking at on Friday, and short enough that a namespace deleted in between has
 * usually stopped mattering.
 */
export const scopeMaxAgeSeconds = 60 * 60 * 24 * 7;

/** Whether a scope means the whole cluster rather than one namespace. */
export function isAllNamespaces(scope: NamespaceScope): boolean {
	return scope === ALL_NAMESPACES;
}

/**
 * The scope a stored cookie value means.
 *
 * Anything unrecognisable reads as the whole cluster, which is the safe end of
 * this: a scope that is wider than intended shows rows from namespaces you did
 * not mean to see, where one that is narrower than intended hides rows you did —
 * and, on the create forms, would offer a namespace nobody picked.
 */
export function readScope(stored: string | undefined): NamespaceScope {
	if (!stored) return ALL_NAMESPACES;
	if (stored === ALL_NAMESPACES) return ALL_NAMESPACES;
	return isNamespaceName(stored) ? stored : ALL_NAMESPACES;
}

/** A DNS-1123 label, which is what Kubernetes accepts as a namespace name. */
export function isNamespaceName(value: string): boolean {
	return value.length <= 63 && /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(value);
}
