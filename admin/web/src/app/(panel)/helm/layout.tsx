import { ShipWheel } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTabs } from "../_components/section-tabs";

const tabs = [
	{ href: "/helm/deployments", label: "Deployments" },
	{ href: "/helm/releases", label: "Releases" },
];

export default function HelmLayout({ children }: { children: React.ReactNode }) {
	return (
		<main className="flex min-h-screen flex-col p-6">
			<PageHeader
				icon={ShipWheel}
				title="Helm"
				description="Deployments are the charts this lab declared — their values live here and are edited here. Releases are what Helm actually has, including anything installed by hand."
			/>
			<SectionTabs tabs={tabs} />
			{children}
		</main>
	);
}
