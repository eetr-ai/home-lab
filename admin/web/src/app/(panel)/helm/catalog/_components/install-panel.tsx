"use client";

import { useState } from "react";
import { ShipWheel } from "lucide-react";
import { installRelease } from "@/app/actions/helm";
import { Checkbox, FormField, Input, Select } from "@/components/ui";
import { CreatePanel } from "../../../_components/create-panel";
import type { HelmChartListing } from "@/lib/api/types";

/**
 * Installing a catalogued chart.
 *
 * Three choices and no values editor. Values are the one thing that could be
 * offered here and deliberately is not: a chart's defaults are the vetted
 * configuration, and a free-form editor at install time is where a release
 * acquires settings nobody reviewed. Changing them is a deliberate act against a
 * release that already exists, not part of creating one.
 */
export function InstallPanel({
	chart,
	namespaces,
	onClose,
}: {
	chart: HelmChartListing | null;
	namespaces: string[];
	onClose: () => void;
}) {
	const [name, setName] = useState("");
	const [namespace, setNamespace] = useState(namespaces[0] ?? "");
	const [version, setVersion] = useState("");
	const [rollbackOnFailure, setRollbackOnFailure] = useState(false);

	function reset() {
		setName("");
		setVersion("");
		setRollbackOnFailure(false);
		onClose();
	}

	return (
		<CreatePanel
			open={chart !== null}
			title={chart ? `Install ${chart.name}` : "Install"}
			icon={ShipWheel}
			submitLabel="Install"
			description="Helm waits for the pods, so this is accepted rather than finished. The release page follows it."
			dirty={name !== "" || version !== ""}
			onClose={reset}
			onSubmit={() =>
				installRelease(namespace, {
					name,
					chart: chart?.name ?? "",
					version,
					rollbackOnFailure,
				})
			}
		>
			<FormField label="Release name" htmlFor="install-name">
				<Input
					id="install-name"
					value={name}
					onChange={(event) => setName(event.target.value)}
					placeholder={chart?.name ?? ""}
					autoComplete="off"
					required
				/>
			</FormField>

			<FormField label="Namespace" htmlFor="install-namespace">
				<Select
					id="install-namespace"
					className="w-full"
					value={namespace}
					onChange={(event) => setNamespace(event.target.value)}
					required
				>
					{namespaces.map((candidate) => (
						<option key={candidate} value={candidate}>
							{candidate}
						</option>
					))}
				</Select>
			</FormField>

			<FormField label="Version" htmlFor="install-version">
				<Select
					id="install-version"
					className="w-full"
					value={version}
					onChange={(event) => setVersion(event.target.value)}
					required
				>
					<option value="">Choose a version</option>
					{(chart?.versions ?? []).map((candidate) => (
						<option key={candidate.version} value={candidate.version}>
							{candidate.version}
							{candidate.appVersion ? ` (app ${candidate.appVersion})` : ""}
						</option>
					))}
				</Select>
			</FormField>

			<Checkbox
				id="install-rollback"
				label="Roll back automatically if it fails"
				hint="Off by default. A failed install that is undone leaves nothing behind and no failed release to read, which makes it harder to find out why."
				checked={rollbackOnFailure}
				onChange={setRollbackOnFailure}
			/>
		</CreatePanel>
	);
}
