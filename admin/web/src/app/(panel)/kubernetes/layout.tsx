import { Boxes } from "lucide-react";
import { Suspense } from "react";
import { PageHeader } from "@/components/ui/page-header";
import { NamespaceSwitcher } from "../_components/namespace-switcher";
import { SectionTabs } from "../_components/section-tabs";

const tabs = [
	{ href: "/kubernetes/secrets", label: "Secrets" },
	{ href: "/kubernetes/workloads", label: "Workloads" },
	{ href: "/kubernetes/pods", label: "Pods" },
	{ href: "/kubernetes/jobs", label: "Jobs" },
	{ href: "/kubernetes/events", label: "Events" },
	{ href: "/kubernetes/nodes", label: "Nodes" },
	{ href: "/kubernetes/storage", label: "Storage" },
];

export default function KubernetesLayout({ children }: { children: React.ReactNode }) {
	return (
		<main className="flex min-h-screen flex-col p-6">
			<PageHeader
				icon={Boxes}
				title="Kubernetes"
				description="What is running on the cluster. Workloads can be restarted and scaled; what they are belongs in this repository's Helm releases."
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
