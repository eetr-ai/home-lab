"use client";

import { createContext, useContext, useTransition } from "react";
import { useRouter } from "next/navigation";
import type { Namespace } from "@/lib/api/types";
import {
	isAllNamespaces,
	scopeCookie,
	scopeMaxAgeSeconds,
	type NamespaceScope,
} from "@/lib/kube/scope";

interface NamespaceScopeValue {
	/**
	 * Every namespace on the cluster, in the order the API listed them, each
	 * carrying the API's own answer about what may be done in it. The whole object
	 * rather than the name: a form choosing where to write something has to offer
	 * only the namespaces the API would accept, and `helmManaged` is that answer.
	 */
	namespaces: Namespace[];
	/** A namespace name, or ALL_NAMESPACES. */
	scope: NamespaceScope;
	isAll: boolean;
	/**
	 * Remember a scope and re-render for it.
	 *
	 * `at` is where to land, for the caller that also has to change the address —
	 * a page reached with a `?namespace=` in it keeps resolving to that namespace,
	 * so choosing another one has to drop the parameter in the same step or the
	 * choice snaps straight back.
	 */
	setScope: (next: NamespaceScope, at?: string) => void;
	/** True while the pages are being re-rendered for a new scope. */
	pending: boolean;
}

const NamespaceScopeContext = createContext<NamespaceScopeValue | null>(null);

/**
 * The namespace every cluster page is scoped by, held once for the whole panel.
 *
 * It is context rather than a query parameter because it is not a property of a
 * page. Each page used to carry its own `?namespace=` and resolve it against the
 * namespace list, falling back to the first one alphabetically — so moving from
 * Namespaces to Secrets silently re-scoped you to whatever sorts first, with
 * nothing on screen saying it had changed. A Secret written into the wrong
 * namespace is what that costs, and it is not a mistake the operator can see
 * afterwards.
 *
 * The value is mirrored into a cookie, which is what lets a Server Component read
 * it. That is the whole reason for the cookie: these pages fetch on the server,
 * so a scope only the browser knew would have nothing to scope. Writing it here
 * rather than through a server action keeps the write and the refresh in one
 * transition — the cookie is set, the route is refreshed, and the pages come back
 * for the namespace that was just chosen.
 */
export function NamespaceScopeProvider({
	namespaces,
	scope,
	children,
}: {
	namespaces: Namespace[];
	scope: NamespaceScope;
	children: React.ReactNode;
}) {
	const router = useRouter();
	const [pending, startTransition] = useTransition();

	function setScope(next: NamespaceScope, at?: string) {
		document.cookie =
			`${scopeCookie}=${encodeURIComponent(next)}; path=/; ` +
			`max-age=${scopeMaxAgeSeconds}; samesite=lax`;
		// Both of these re-render the server components with the cookie that was
		// just written; the replace additionally leaves the old address behind.
		startTransition(() => (at === undefined ? router.refresh() : router.replace(at)));
	}

	return (
		<NamespaceScopeContext.Provider
			value={{ namespaces, scope, isAll: isAllNamespaces(scope), setScope, pending }}
		>
			{children}
		</NamespaceScopeContext.Provider>
	);
}

/**
 * The current scope, for a client component that needs to show it or change it.
 *
 * Throws outside the provider rather than returning a default. A create form that
 * silently believed it was in "all namespaces" is the failure this whole module
 * exists to prevent, and a hook that quietly invents a scope is how it would come
 * back.
 */
export function useNamespaceScope(): NamespaceScopeValue {
	const value = useContext(NamespaceScopeContext);
	if (value === null) {
		throw new Error("useNamespaceScope must be used inside a NamespaceScopeProvider");
	}
	return value;
}
