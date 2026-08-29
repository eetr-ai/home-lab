import { Boxes } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTabs } from "../_components/section-tabs";

const tabs = [
	{ href: "/kubernetes/namespaces", label: "Namespaces" },
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
				description="What is running on the cluster. Workloads can be restarted and scaled; what they are belongs in this repository's Helm releases."
			/>
			<SectionTabs tabs={tabs} />
			{children}
		</main>
	);
}
