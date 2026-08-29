"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { ShipWheel } from "lucide-react";
import { declareDeployment } from "@/app/actions/helm";
import { FormField, Input, Label, Select } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";
import { YamlEditor } from "@/components/editor/yaml-editor";
import { useChartVersions } from "./use-chart-versions";
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
	const [chartRef, setChartRef] = useState("");
	const [name, setName] = useState("");
	const [namespace, setNamespace] = useState("");
	const [version, setVersion] = useState("");
	const [values, setValues] = useState(startingValues);
	const router = useRouter();

	const versions = useChartVersions(open ? chartRef : "");

	function reset() {
		setChartRef("");
		setName("");
		setNamespace("");
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
			dirty={chartRef !== "" || name !== "" || values !== startingValues}
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
					onChange={(event) => setChartRef(event.target.value)}
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
				{/* A picker when the registry answered, a text field when it did not.
				    An unreachable registry is a reason to type the version yourself,
				    not a reason to be unable to declare anything. */}
				{versions.offered.length > 0 ? (
					<Select
						id="chart-version"
						value={version}
						onChange={(event) => setVersion(event.target.value)}
						required
					>
						<option value="">Choose a version</option>
						{versions.offered.map((offered) => (
							<option key={offered.version} value={offered.version}>
								{offered.appVersion ? `${offered.version} (app ${offered.appVersion})` : offered.version}
							</option>
						))}
					</Select>
				) : (
					<Input
						id="chart-version"
						value={version}
						onChange={(event) => setVersion(event.target.value)}
						placeholder="6.9.2"
						autoComplete="off"
						spellCheck={false}
						required
					/>
				)}
				<Hint>{versions.hint}</Hint>
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
