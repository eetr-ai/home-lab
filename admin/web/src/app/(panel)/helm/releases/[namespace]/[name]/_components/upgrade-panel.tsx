"use client";

import { useState } from "react";
import { History } from "lucide-react";
import { upgradeRelease } from "@/app/actions/helm";
import { Checkbox, FormField, Select } from "@/components/ui";
import { CreatePanel } from "../../../../../_components/create-panel";
import type { HelmReleaseDetail } from "@/lib/api/types";

/**
 * Choosing a version to move a release to.
 *
 * Values are deliberately not editable here. Omitting them tells the API to keep
 * what the release already has, which is the same thing a pipeline does — one way
 * for values to change beats two that have to agree, and an editor that
 * round-trips the stored values would let a stray keystroke rewrite the
 * configuration of something that was only meant to move a version.
 */
export function UpgradePanel({
	open,
	release,
	versions,
	onClose,
}: {
	open: boolean;
	release: HelmReleaseDetail;
	versions: string[];
	onClose: () => void;
}) {
	const [version, setVersion] = useState("");
	const [rollbackOnFailure, setRollbackOnFailure] = useState(false);

	function reset() {
		setVersion("");
		setRollbackOnFailure(false);
		onClose();
	}

	return (
		<CreatePanel
			open={open}
			title={`Upgrade ${release.name}`}
			icon={History}
			submitLabel="Upgrade"
			description="The release keeps the values it already has. Helm waits for the pods, so this is accepted rather than finished — the page follows it."
			dirty={version !== ""}
			onClose={reset}
			onSubmit={() =>
				upgradeRelease(release.namespace, release.name, { version, rollbackOnFailure })
			}
		>
			<FormField label="Version" htmlFor="upgrade-version">
				<Select
					id="upgrade-version"
					className="w-full"
					value={version}
					onChange={(event) => setVersion(event.target.value)}
					required
				>
					<option value="">Choose a version</option>
					{versions.map((candidate) => (
						<option key={candidate} value={candidate}>
							{candidate}
							{candidate === release.chartVersion ? " (current)" : ""}
						</option>
					))}
				</Select>
			</FormField>

			<Checkbox
				id="upgrade-rollback"
				label="Roll back automatically if it fails"
				hint="Off by default. With it on a failed upgrade ends up deployed again — on the chart it started from — which reads as success to anything checking only the status."
				checked={rollbackOnFailure}
				onChange={setRollbackOnFailure}
			/>
		</CreatePanel>
	);
}
