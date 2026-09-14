"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { ShipWheel } from "lucide-react";
import { declareDeployment } from "@/app/actions/helm";
import { FormField, Input, Label, Select } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";
import { useNamespaceScope } from "../../../_components/namespace-scope";
import { YamlEditor } from "@/components/editor/yaml-editor";
import { ChartVersionField } from "./chart-version-field";
import type { Namespace } from "@/lib/api/types";

/** What a values file usually starts as, so the editor is never a blank void. */
const startingValues = `# Values for this release. Anything you leave out keeps the chart's default.
`;

/**
 * Declaring a deployment: a chart, where it goes, and what it is configured with.
 *
 * Declaring is not deploying, and the submit label says so. Writing the record
 * first means a values file you are halfway through is a saved draft rather than
 * a failed install — and it means the version and the values are reviewable
 * before anything reaches the cluster.
 */
export function DeclarePanel({
	open,
	namespaces,
	onClose,
}: {
	open: boolean;
	namespaces: Namespace[];
	onClose: () => void;
}) {
	// Pre-filled from the panel's namespace scope where that names one the API
	// would accept, and empty otherwise — including when the scope is every
	// namespace, which is the case a form must not answer on the operator's
	// behalf. `suggested` is also what `dirty` compares against, so a pre-filled
	// namespace is not by itself an unsaved change.
	const { scope } = useNamespaceScope();
	const suggested = namespaces.some((one) => one.name === scope) ? scope : "";

	const [chartRef, setChartRef] = useState("");
	const [name, setName] = useState("");
	const [namespace, setNamespace] = useState(suggested);
	const [version, setVersion] = useState("");
	const [values, setValues] = useState(startingValues);
	const router = useRouter();

	// Changing the chart clears the version. Without this, picking 6.9.2 for one
	// chart and then editing the reference submits 6.9.2 for a chart that may not
	// publish it — and the picker would be showing a value that is not in it.
	function changeChartRef(value: string) {
		setChartRef(value);
		setVersion("");
	}

	function reset() {
		setChartRef("");
		setName("");
		setNamespace(suggested);
		setVersion("");
		setValues(startingValues);
		onClose();
	}

	return (
		<CreatePanel
			open={open}
			title="New deployment"
			icon={ShipWheel}
			submitLabel="Declare"
			description="Records the chart and its values. Nothing reaches the cluster until you roll it out."
			// Every field, not just the obvious ones. CreatePanel skips its discard
			// guard when this is false and then clears the form, so a namespace or
			// a version chosen and nothing else typed would vanish without asking.
			dirty={
				chartRef !== "" ||
				name !== "" ||
				namespace !== suggested ||
				version !== "" ||
				values !== startingValues
			}
			onClose={reset}
			onSubmit={async () => {
				const result = await declareDeployment({
					namespace,
					name,
					chartRef: chartRef.trim(),
					version,
					valuesYaml: values,
				});
				if (result.ok) router.push(`/helm/deployments/${result.data.id}`);
				return result;
			}}
		>
			<FormField label="Chart reference" htmlFor="chart-ref">
				<Input
					id="chart-ref"
					value={chartRef}
					onChange={(event) => changeChartRef(event.target.value)}
					placeholder="oci://ghcr.io/stefanprodan/charts/podinfo"
					autoComplete="off"
					spellCheck={false}
					required
				/>
				<Hint>
					oci://ghcr.io/org/charts/podinfo, or an https chart repository ending in the
					chart name
				</Hint>
			</FormField>

			<FormField label="Version" htmlFor="chart-version">
				{/* Follows the reference as it is typed: here the reference is being
				    chosen, so the versions it offers are part of choosing it. */}
				<ChartVersionField
					id="chart-version"
					reference={open ? chartRef : ""}
					value={version}
					onChange={setVersion}
					follow
				/>
			</FormField>

			<FormField label="Release name" htmlFor="release-name">
				<Input
					id="release-name"
					value={name}
					onChange={(event) => setName(event.target.value)}
					placeholder={defaultName(chartRef)}
					autoComplete="off"
					spellCheck={false}
					required
				/>
				<Hint>What Helm will call it in the namespace</Hint>
			</FormField>

			<FormField label="Namespace" htmlFor="namespace">
				<Select
					id="namespace"
					value={namespace}
					onChange={(event) => setNamespace(event.target.value)}
					required
				>
					<option value="">Choose a namespace</option>
					{namespaces.map((one) => (
						<option key={one.name} value={one.name}>
							{one.name}
						</option>
					))}
				</Select>
			</FormField>

			<div>
				{/* Not a FormField: that associates a label with one control by id,
				    and the editor is a div of many. The label is a plain heading and
				    the editor is reachable by tab. */}
				<Label>Values</Label>
				<YamlEditor value={values} onChange={setValues} minHeight="16rem" />
				<Hint>YAML. Comments are kept exactly as you write them.</Hint>
			</div>
		</CreatePanel>
	);
}

/** A line of guidance under a control. Nothing renders for an empty one. */
function Hint({ children }: { children?: React.ReactNode }) {
	if (!children) return null;
	return <p className="mt-1 text-xs text-muted-foreground">{children}</p>;
}

/** The chart's own name, which is what most releases end up called. */
function defaultName(reference: string): string {
	const trimmed = reference.trim().replace(/\/+$/, "");
	const last = trimmed.slice(trimmed.lastIndexOf("/") + 1);
	return last && !last.includes(":") ? last : "podinfo";
}
