import { RedisIcon } from "@/components/ui/redis-icon";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTabs } from "../_components/section-tabs";

const tabs = [
	{ href: "/redis/overview", label: "Overview" },
	{ href: "/redis/keys", label: "Keys" },
	{ href: "/redis/users", label: "Users" },
	{ href: "/redis/access", label: "Access" },
];

export default function RedisLayout({ children }: { children: React.ReactNode }) {
	return (
		<main className="flex min-h-screen flex-col p-6">
			<PageHeader
				icon={RedisIcon}
				title="Redis"
				description="The platform's shared Redis: what is in it, who may authenticate, and which namespaces may connect."
			/>
			<SectionTabs tabs={tabs} />
			{children}
		</main>
	);
}
