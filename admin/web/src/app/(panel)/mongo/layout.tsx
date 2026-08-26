import { Leaf } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTabs } from "../_components/section-tabs";

const tabs = [
	{ href: "/mongo/databases", label: "Databases" },
	{ href: "/mongo/collections", label: "Collections" },
	{ href: "/mongo/users", label: "Users" },
];

export default function MongoLayout({ children }: { children: React.ReactNode }) {
	return (
		<main className="flex min-h-screen flex-col p-6">
			<PageHeader
				icon={Leaf}
				title="MongoDB"
				description="The databases, collections, and users on the host's MongoDB server."
			/>
			<SectionTabs tabs={tabs} />
			{children}
		</main>
	);
}
