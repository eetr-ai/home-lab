import { Database } from "lucide-react";
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
		<main className="flex min-h-screen flex-col p-6">
			<PageHeader
				icon={Database}
				title="PostgreSQL"
				description="The databases, roles, and extensions on the host's PostgreSQL server."
			/>
			<SectionTabs tabs={tabs} />
			{children}
		</main>
	);
}
