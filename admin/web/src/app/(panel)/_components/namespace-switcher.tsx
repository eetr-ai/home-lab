"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { ChevronDown, Layers, Settings } from "lucide-react";
import { buttonVariants, cn, Popover, Spinner } from "@/components/ui";
import { ComboboxList } from "@/components/ui/combobox-list";
import { ALL_NAMESPACES, isAllNamespaces, resolveScope } from "@/lib/kube/scope";
import { namespaceScopeFor } from "@/lib/kube/scoped-routes";
import { useNamespaceScope } from "./namespace-scope";

/** Where the gear goes: the page that creates and deletes namespaces. */
const MANAGE_HREF = "/kubernetes/namespaces";

/** One row in the list: a namespace, or the whole cluster pinned above them. */
interface ScopeOption {
	name: string;
	all: boolean;
	helmManaged: boolean;
}

const allOption: ScopeOption = { name: ALL_NAMESPACES, all: true, helmManaged: false };

function optionText(option: ScopeOption): string {
	return option.all ? "All namespaces" : option.name;
}

function optionKey(option: ScopeOption): string {
	return option.name;
}

/**
 * The control that chooses which namespace the panel is looking at, in the
 * top-right corner of the section header.
 *
 * One control over one shared value, so every list it applies to agrees about
 * what is being shown. It lives in the section layout rather than on each page,
 * and hides itself where the scope does not apply — detail pages, which are about
 * one object in one namespace, and the cluster-wide listings. Which pages those
 * are is `namespaceScopeFor`'s answer, not this component's.
 *
 * A layout does not receive the page's search parameters, so the scope shown is
 * resolved here the same way the server resolves it: a `?namespace=` in the
 * address wins over the remembered one. That keeps the name on the button and
 * the rows under it in agreement.
 *
 * "All namespaces" is a real choice rather than the absence of one. It is also
 * the default, and that is the point: falling back to a namespace nobody picked
 * is what put a Secret in the wrong one.
 */
export function NamespaceSwitcher() {
	const { namespaces, scope: remembered, setScope, pending } = useNamespaceScope();
	const pathname = usePathname();
	const params = useSearchParams();
	const [open, setOpen] = useState(false);
	const trigger = useRef<HTMLButtonElement>(null);

	const use = namespaceScopeFor(pathname);
	const scope = resolveScope(params.get("namespace"), remembered);

	// Choosing drops any `?namespace=` from the address as it goes. That parameter
	// wins over the remembered scope, so leaving it in place would have the page
	// resolve straight back to it and the switcher would appear not to work.
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
		// Only where the scope is on screen: a detail page reached with a
		// `?namespace=` is not choosing one.
		if (use === "none") return;
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
		// then finding the switcher naming a different namespace than the rows
		// beneath it is exactly the confusion this module exists to remove.
		if (scope !== remembered) setScope(scope);
		// `choose` and `setScope` close over the router and the transition starter,
		// both of which Next keeps identical across renders.
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [use, known, scope, remembered]);

	// Every namespace, protected ones included: reading is offered everywhere,
	// and it is writing that is narrowed — by the create forms, which offer only
	// what the API would accept.
	const options = useMemo<ScopeOption[]>(
		() => [
			allOption,
			...namespaces.map((one) => ({ name: one.name, all: false, helmManaged: one.helmManaged })),
		],
		[namespaces],
	);

	if (use === "none") return null;

	const everywhere = isAllNamespaces(scope);
	// The Secrets tab cannot list the whole cluster, so "All" there is a choice
	// still to be made, and the button says so instead of naming it.
	const unchosen = use === "required" && everywhere;
	const selected = options.find((option) => option.name === scope) ?? allOption;

	function pick(option: ScopeOption) {
		setOpen(false);
		trigger.current?.focus();
		if (option.name !== scope) choose(option.name);
	}

	return (
		<>
			<button
				ref={trigger}
				type="button"
				disabled={namespaces.length === 0}
				onClick={() => setOpen((wasOpen) => !wasOpen)}
				aria-haspopup="dialog"
				aria-expanded={open}
				aria-label={`Namespace: ${unchosen ? "none chosen" : optionText(selected)}`}
				className={cn(
					buttonVariants.secondary,
					"flex max-w-72 items-center gap-2",
					unchosen && "border-warning-border bg-warning-bg text-warning-fg",
				)}
			>
				{pending ? (
					<Spinner className="text-muted-foreground" />
				) : (
					<Layers
						className={cn("h-4 w-4 shrink-0", !unchosen && "text-muted-foreground")}
						aria-hidden
					/>
				)}
				<span className="min-w-0 truncate">
					{unchosen ? "Choose a namespace" : optionText(selected)}
				</span>
				<ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden />
			</button>

			<Popover
				open={open}
				onRequestClose={() => setOpen(false)}
				anchor={trigger}
				title="Namespace"
				width="sm"
			>
				<div className="flex items-center justify-between border-b border-border px-3 py-2">
					<h3 className="text-sm font-medium">Namespace</h3>
					<Link
						href={MANAGE_HREF}
						onClick={() => setOpen(false)}
						aria-label="Manage namespaces"
						title="Manage namespaces"
						className="rounded-full p-1.5 text-muted-foreground hover:bg-surface-hover hover:text-foreground"
					>
						<Settings className="h-4 w-4" aria-hidden />
					</Link>
				</div>

				<ComboboxList
					items={options}
					selected={selected}
					onChoose={pick}
					toKey={optionKey}
					toText={optionText}
					renderRow={(option) => (
						<span className="flex items-center gap-2">
							<span className={cn("truncate", option.all && "font-medium")}>
								{optionText(option)}
							</span>
							{option.helmManaged ? (
								<span className="rounded-chip bg-surface-sunken px-1.5 py-0.5 text-xs text-muted-foreground">
									Helm
								</span>
							) : null}
						</span>
					)}
					label="Namespaces"
				/>

				<div className="border-t border-border px-3 py-2">
					<Link
						href={MANAGE_HREF}
						onClick={() => setOpen(false)}
						className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground"
					>
						<Settings className="h-3.5 w-3.5" aria-hidden />
						Manage namespaces
					</Link>
				</div>
			</Popover>
		</>
	);
}
