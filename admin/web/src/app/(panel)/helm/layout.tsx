import { ShipWheel } from "lucide-react";
import { Suspense } from "react";
import { PageHeader } from "@/components/ui/page-header";
import { NamespaceSwitcher } from "../_components/namespace-switcher";
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
				action={
					// useSearchParams in a layout needs a boundary of its own.
					<Suspense fallback={null}>
						<NamespaceSwitcher />
					</Suspense>
				}
			/>
			<SectionTabs tabs={tabs} />
			{children}
		</main>
	);
}
