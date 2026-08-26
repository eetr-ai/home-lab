import { Boxes } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTabs } from "../_components/section-tabs";

const tabs = [
	{ href: "/kubernetes/workloads", label: "Workloads" },
	{ href: "/kubernetes/pods", label: "Pods" },
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
				description="What is running on the cluster. Read-only — changes belong in this repository's Helm releases."
			/>
			<SectionTabs tabs={tabs} />
			{children}
		</main>
	);
}
