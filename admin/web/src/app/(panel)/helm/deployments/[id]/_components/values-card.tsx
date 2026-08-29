"use client";

import { useState } from "react";
import { Rocket, Save } from "lucide-react";
import { addDeploymentVersion, rolloutDeployment } from "@/app/actions/helm";
import { Banner, Button, Card, Checkbox } from "@/components/ui";
import { YamlEditor } from "@/components/editor/yaml-editor";
import type { HelmDeploymentDetail, HelmDeploymentVersion } from "@/lib/api/types";

/**
 * The values, and the two things you can do with them.
 *
 * Saving and rolling out are separate buttons because they are separate
 * decisions: saving writes a version and touches nothing, rolling out puts a
 * version on the cluster. Collapsing them into one "Apply" would make every typo
 * a deploy.
 *
 * Editing an older version is allowed and produces a new version rather than
 * changing that one — the history is append-only, so "go back to what we had on
 * Tuesday" is a save away and leaves the record of what happened intact.
 */
export function ValuesCard({
	deployment,
	editing,
	onEditingChange,
}: {
	deployment: HelmDeploymentDetail;
	/** The version whose values are in the editor. */
	editing: number;
	onEditingChange: (version: number) => void;
}) {
	// The version this page has just written, kept until the refreshed props
	// carry it. Saving tells the parent to show version N+1, but the props still
	// describe the world before the save — so without this, `shown` falls back to
	// deployment.current and the editor replaces what was just saved with an
	// older version's values until the refresh lands. Most visible when saving
	// from an older version, where the fallback is a different document entirely.
	const [saved, setSaved] = useState<HelmDeploymentVersion | null>(null);

	const shown =
		deployment.versions.find((one) => one.version === editing) ??
		(saved?.version === editing ? saved : null) ??
		deployment.current;

	const [draft, setDraft] = useState(shown.valuesYaml);
	const [chartVersion, setChartVersion] = useState(shown.chartVersion);
	const [shownVersion, setShownVersion] = useState(shown.version);
	const [rollbackOnFailure, setRollbackOnFailure] = useState(false);
	const [error, setError] = useState<string | null>(null);
	const [busy, setBusy] = useState<"saving" | "rolling-out" | null>(null);

	// Switching to another version replaces the draft. Done during render rather
	// than in an effect so the editor never shows one version's values under
	// another's number, not even for a frame.
	if (shown.version !== shownVersion) {
		setShownVersion(shown.version);
		setDraft(shown.valuesYaml);
		setChartVersion(shown.chartVersion);
	}

	const dirty = draft !== shown.valuesYaml || chartVersion !== shown.chartVersion;
	// An empty chart version would be sent as "" and refused by the API. The
	// declare form makes this field required; this one has to say so too.
	const versioned = chartVersion.trim() !== "";

	async function save() {
		setError(null);
		setBusy("saving");
		const result = await addDeploymentVersion(deployment.id, {
			version: chartVersion,
			valuesYaml: draft,
		});
		setBusy(null);
		if (!result.ok) {
			setError(result.error);
			return;
		}
		setSaved(result.data);
		onEditingChange(result.data.version);
	}

	async function rollout() {
		setError(null);
		setBusy("rolling-out");
		// A dirty editor is saved first, so the button never deploys something
		// other than what is on screen.
		if (dirty) {
			const appended = await addDeploymentVersion(deployment.id, {
				version: chartVersion,
				valuesYaml: draft,
			});
			if (!appended.ok) {
				setBusy(null);
				setError(appended.error);
				return;
			}
			setSaved(appended.data);
			onEditingChange(appended.data.version);
		}

		const result = await rolloutDeployment(deployment.id, { rollbackOnFailure });
		setBusy(null);
		if (!result.ok) setError(result.error);
	}

	return (
		<Card padding="md">
			<div className="mb-3 flex flex-wrap items-center justify-between gap-3">
				<div className="flex items-baseline gap-2">
					<h2 className="text-sm font-medium">Values</h2>
					<span className="text-xs text-muted-foreground">
						version {shown.version}
						{shown.version !== deployment.current.version ? " (not the newest)" : ""}
						{dirty ? " · unsaved" : ""}
					</span>
				</div>

				<div className="flex items-center gap-2">
					<label className="flex items-center gap-2 text-xs text-muted-foreground">
						Chart version
						<input
							value={chartVersion}
							onChange={(event) => setChartVersion(event.target.value)}
							spellCheck={false}
							autoComplete="off"
							className="w-28 rounded-control border border-border px-2 py-1 text-xs"
						/>
					</label>
					<Button
						variant="secondary"
						icon={Save}
						onClick={save}
						disabled={!dirty || !versioned || busy !== null}
						loading={busy === "saving"}
					>
						Save version
					</Button>
					<Button
						icon={Rocket}
						onClick={rollout}
						disabled={!versioned || busy !== null}
						loading={busy === "rolling-out"}
					>
						Roll out
					</Button>
				</div>
			</div>

			<Banner variant="error" message={error} />

			<YamlEditor value={draft} onChange={setDraft} minHeight="22rem" />

			<div className="mt-3">
				<Checkbox
					checked={rollbackOnFailure}
					onChange={setRollbackOnFailure}
					label="Roll back automatically if it fails"
					hint="Off by default: with it on, a failed deploy is undone and the release reports deployed on the previous version, which reads as success to anything polling."
				/>
			</div>
		</Card>
	);
}
