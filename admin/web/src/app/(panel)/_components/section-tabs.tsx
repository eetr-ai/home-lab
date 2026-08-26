"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { activeHref } from "@/lib/nav/active";

/**
 * A section's tab strip. The section layout owns it, and each tab is a route
 * segment rather than client state — so a tab is bookmarkable, survives a reload,
 * and the back button moves between them the way it should.
 *
 * Which tab is lit comes from the longest matching href, so a child route keeps
 * its parent tab active. See src/lib/nav/active.ts.
 */
export function SectionTabs({ tabs }: { tabs: { href: string; label: string }[] }) {
	const pathname = usePathname();
	const active = activeHref(
		tabs.map((tab) => tab.href),
		pathname,
	);

	return (
		<nav className="mb-6 flex flex-wrap gap-1 border-b border-border" aria-label="Section">
			{tabs.map(({ href, label }) => (
				<Link
					key={href}
					href={href}
					aria-current={href === active ? "page" : undefined}
					className={`-mb-px border-b-2 px-3 py-2 text-sm font-medium transition-colors ${
						href === active
							? "border-brand text-foreground"
							: "border-transparent text-muted-foreground hover:text-foreground"
					}`}
				>
					{label}
				</Link>
			))}
		</nav>
	);
}
