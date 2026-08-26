import { listPods } from "@/app/actions/kube";
import { ScopePicker } from "../../_components/scope-picker";
import { resolveNamespace } from "../_components/namespace-scope";
import { PodList } from "./_components/pod-list";

export const dynamic = "force-dynamic";

export default async function PodsPage({
	searchParams,
}: {
	searchParams: Promise<{ namespace?: string }>;
}) {
	const { namespace: requested } = await searchParams;
	const { namespaces, selected, error } = await resolveNamespace(requested);
	const pods = selected ? await listPods(selected) : null;

	return (
		<>
			<ScopePicker label="Namespace" param="namespace" options={namespaces} selected={selected} />
			<PodList
				namespace={selected ?? ""}
				pods={pods?.ok ? pods.data : []}
				error={error ?? (pods && !pods.ok ? pods.error : null)}
			/>
		</>
	);
}
