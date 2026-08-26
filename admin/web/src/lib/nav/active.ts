import type { LucideIcon } from "lucide-react";

/** One entry in the sidebar or a section's tab strip. */
export interface NavItem {
	href: string;
	label: string;
	icon: LucideIcon;
}

/**
 * Which of `hrefs` the current path is inside — the longest one that matches.
 *
 * Longest wins because navigation nests: `/postgres` and `/postgres/roles` are
 * both prefixes of `/postgres/roles`, and highlighting the first would light up
 * the parent while the operator is looking at the child. This is the rule behind
 * both the sidebar and the section tabs, which is why it lives here rather than
 * inside either of them.
 *
 * Matching respects segment boundaries: `/postgres` does not contain
 * `/postgres-archive`, however much the strings look alike.
 */
export function activeHref(hrefs: string[], pathname: string): string | undefined {
	let best: string | undefined;
	for (const href of hrefs) {
		if (pathname !== href && !pathname.startsWith(`${href}/`)) continue;
		if (best === undefined || href.length > best.length) best = href;
	}
	return best;
}
