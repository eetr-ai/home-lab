"use client";

import { useEffect } from "react";
import { usePathname, useSearchParams } from "next/navigation";
import { Select } from "@/components/ui/select";
import { Spinner } from "@/components/ui/spinner";
import { ALL_NAMESPACES, isAllNamespaces } from "@/lib/kube/scope";
import { useNamespaceScope } from "./namespace-scope";

/**
 * The control that chooses which namespace the panel is looking at.
 *
 * One control over one shared value, so every list it applies to agrees about
 * what is being shown. It is rendered by the pages it scopes rather than by the
 * section layout, because a detail page is about one object in one namespace and
 * a picker above it would look like it applied.
 *
 * "All namespaces" is a real choice here rather than the absence of one. It is
 * also the default, and that is the point: falling back to a namespace nobody
 * picked is what put a Secret in the wrong one.
 */
export function NamespacePicker({
	/**
	 * The scope the page actually rendered. Usually the shared one, and not when
	 * a `?namespace=` in the address named something else — a link out of a
	 * workload, a bookmark, the agent.
	 */
	scope,
	/**
	 * Set on a page that cannot answer for the whole cluster. The Secrets tab is
	 * the one: the panel's grant for Secrets is bound per namespace, so there is
	 * no cluster-wide listing to offer and the choice has to be made.
	 */
	requireOne = false,
}: {
	scope: string;
	requireOne?: boolean;
}) {
	const { namespaces, scope: remembered, setScope, pending } = useNamespaceScope();
	const pathname = usePathname();
	const params = useSearchParams();

	// Choosing drops any `?namespace=` from the address as it goes. That parameter
	// wins over the remembered scope, so leaving it in place would have the page
	// resolve straight back to it and the dropdown would appear not to work.
	function choose(next: string) {
		const query = new URLSearchParams(params.toString());
		if (!query.has("namespace")) {
			setScope(next);
			return;
		}
		query.delete("namespace");
		const rest = query.toString();
		setScope(next, rest === "" ? pathname : `${pathname}?${rest}`);
	}

	// Whether the scope on screen is a namespace that still exists. Only asked
	// where the namespace list was read at all: an empty list means the read
	// failed, and treating that as "every namespace has been deleted" would throw
	// away the operator's scope over a transient failure.
	const known =
		namespaces.length === 0 || isAllNamespaces(scope) || namespaces.some((one) => one.name === scope);

	useEffect(() => {
		// A namespace that has been deleted since it was named falls back to the
		// whole cluster. Through `choose`, so the address that named it is dropped
		// too — otherwise the page would resolve straight back to it and the two
		// corrections would take turns undoing each other.
		if (!known) {
			choose(ALL_NAMESPACES);
			return;
		}
		// Otherwise the page's scope is the one on screen, so the remembered one
		// follows it rather than the other way round. Arriving through a link and
		// then finding the picker naming a different namespace than the rows
		// beneath it is exactly the confusion this module exists to remove.
		if (scope !== remembered) setScope(scope);
		// `choose` and `setScope` close over the router and the transition starter,
		// both of which Next keeps identical across renders.
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [known, scope, remembered]);

	return (
		<div className="mb-4 flex items-center gap-2">
			<label className="text-sm text-muted-foreground" htmlFor="namespace-scope">
				Namespace
			</label>
			<Select
				id="namespace-scope"
				value={scope}
				disabled={pending || namespaces.length === 0}
				onChange={(event) => choose(event.target.value)}
			>
				{/* Offered even where it cannot be listed, so the choice does not
				    disappear from under an operator who wants to go back to it. The
				    page says what it needs instead. */}
				<option value={ALL_NAMESPACES}>
					{requireOne ? "All namespaces — choose one" : "All namespaces"}
				</option>
				{/* Every namespace, protected ones included: reading is offered
				    everywhere, and it is writing that is narrowed — by the create
				    forms, which offer only what the API would accept. */}
				{namespaces.map((one) => (
					<option key={one.name} value={one.name}>
						{one.name}
					</option>
				))}
			</Select>
			{pending ? <Spinner className="text-muted-foreground" /> : null}
		</div>
	);
}
