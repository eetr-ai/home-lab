import { PostgresIcon } from "@/components/ui/postgres-icon";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTabs } from "../_components/section-tabs";

const tabs = [
	{ href: "/postgres/databases", label: "Databases" },
	{ href: "/postgres/roles", label: "Roles" },
	{ href: "/postgres/extensions", label: "Extensions" },
	{ href: "/postgres/query", label: "Query" },
];

/**
 * The PostgreSQL section. The layout owns the heading and the tab strip; each tab
 * is a route segment below it, so the browser's history and the address bar both
 * describe where you are.
 */
export default function PostgresLayout({ children }: { children: React.ReactNode }) {
	return (
		// Bounded to the viewport, not min-h-screen: the header and tabs stay put
		// while the tab below them owns its own scrolling. The query console needs
		// that — its schema tree pins and scrolls on its own, the way the nav does —
		// and the list tabs simply scroll their table inside the same frame.
		<main className="flex h-dvh flex-col p-6">
			<PageHeader
				icon={PostgresIcon}
				title="PostgreSQL"
				description="The databases, roles, and extensions on the host's PostgreSQL server."
			/>
			<SectionTabs tabs={tabs} />
			<div className="flex min-h-0 flex-1 flex-col overflow-y-auto">{children}</div>
		</main>
	);
}
