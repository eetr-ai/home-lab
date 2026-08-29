import { ShipWheel } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTabs } from "../_components/section-tabs";

const tabs = [
	{ href: "/helm/releases", label: "Releases" },
	{ href: "/helm/catalog", label: "Catalog" },
];

export default function HelmLayout({ children }: { children: React.ReactNode }) {
	return (
		<main className="flex min-h-screen flex-col p-6">
			<PageHeader
				icon={ShipWheel}
				title="Helm"
				description="What is installed in the namespaces this lab manages, and what it can install. Helm's own storage is the source of truth — a release installed by hand appears here too."
			/>
			<SectionTabs tabs={tabs} />
			{children}
		</main>
	);
}
