import { ShipWheel } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTabs } from "../_components/section-tabs";

const tabs = [
	{ href: "/helm/dashboard", label: "Dashboard" },
	{ href: "/helm/deployments", label: "Deployments" },
];

export default function HelmLayout({ children }: { children: React.ReactNode }) {
	return (
		<main className="flex min-h-screen flex-col p-6">
			<PageHeader
				icon={ShipWheel}
				title="Helm"
				description="The dashboard is what Helm actually has, including anything installed by hand. Deployments are the charts this lab declared — their values live there and are edited there."
			/>
			<SectionTabs tabs={tabs} />
			{children}
		</main>
	);
}
