"use client";

import { useState } from "react";
import { Layers } from "lucide-react";
import { createNamespace } from "@/app/actions/kube";
import { Checkbox, FormField, Input } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";

/** The two labels a namespace needs to be reachable, and what each one buys. */
const ACCESS_LABELS = [
	{
		key: "home-lab.example/gateway-access",
		label: "Routable through the gateway",
		hint: "Without this the Gateway will not accept an HTTPRoute from the namespace.",
	},
	{
		key: "home-lab.example/redis-access",
		label: "Allowed to reach Redis",
		hint: "The platform Redis refuses connections from namespaces without it.",
	},
] as const;

/**
 * Creating a namespace is two decisions: what to call it, and what it is allowed
 * to reach.
 *
 * The access labels are offered as checkboxes rather than a free-form label
 * editor. Both are enforced elsewhere — one by the Gateway's namespace selector,
 * one by a NetworkPolicy — so a namespace created without the one it needs looks
 * fine and fails later, and picking them here is the difference between that and
 * a working namespace. Everything else the panel stamps on is not the operator's
 * choice: Pod Security enforcement, who manages it, and the Helm marker are
 * applied by the API over whatever is sent.
 */
export function CreateNamespacePanel({
	open,
	onClose,
}: {
	open: boolean;
	onClose: () => void;
}) {
	const [name, setName] = useState("");
	const [access, setAccess] = useState<Record<string, boolean>>({});

	function reset() {
		setName("");
		setAccess({});
		onClose();
	}

	const labels = Object.fromEntries(
		Object.entries(access)
			.filter(([, on]) => on)
			.map(([key]) => [key, "true"]),
	);

	return (
		<CreatePanel
			open={open}
			title="New namespace"
			icon={Layers}
			description="Names follow Kubernetes' DNS label rules: lowercase letters, digits, and hyphens. A new namespace is set up for Helm straight away, so you can deploy into it without reinstalling anything."
			dirty={name !== "" || Object.keys(labels).length > 0}
			onClose={reset}
			onSubmit={() => createNamespace({ name, ...(Object.keys(labels).length ? { labels } : {}) })}
		>
			<FormField label="Name" htmlFor="namespace-name">
				<Input
					id="namespace-name"
					value={name}
					onChange={(event) => setName(event.target.value)}
					placeholder="apps"
					autoComplete="off"
					required
				/>
			</FormField>

			{ACCESS_LABELS.map((entry) => (
				<Checkbox
					key={entry.key}
					id={entry.key}
					label={entry.label}
					hint={entry.hint}
					checked={access[entry.key] ?? false}
					onChange={(checked) => setAccess((current) => ({ ...current, [entry.key]: checked }))}
				/>
			))}
		</CreatePanel>
	);
}
