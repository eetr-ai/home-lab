"use client";

import { useState } from "react";
import { Rocket, Save } from "lucide-react";
import { addDeploymentVersion, rolloutDeployment } from "@/app/actions/helm";
import { Banner, Button, Card, Checkbox } from "@/components/ui";
import { YamlEditor } from "@/components/editor/yaml-editor";
import type { HelmDeploymentDetail } from "@/lib/api/types";

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
	const shown = deployment.versions.find((one) => one.version === editing) ?? deployment.current;

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
	const newest = deployment.current.version;
	const older = shown.version !== newest;

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
		onEditingChange(result.data.version);
	}

	async function rollout() {
		setError(null);
		setBusy("rolling-out");

		// Whichever version is on screen is the one that gets deployed, and it is
		// named explicitly. Leaving it out asks the API for "the newest", which is
		// not the same thing the moment somebody opens an older version from the
		// history — the button would then deploy something other than what they
		// were reading, which is the one thing it must never do.
		let version = shown.version;

		// A dirty editor is saved first, and what that produces is the new newest.
		if (dirty) {
			const saved = await addDeploymentVersion(deployment.id, {
				version: chartVersion,
				valuesYaml: draft,
			});
			if (!saved.ok) {
				setBusy(null);
				setError(saved.error);
				return;
			}
			version = saved.data.version;
			onEditingChange(version);
		}

		const result = await rolloutDeployment(deployment.id, { version, rollbackOnFailure });
		setBusy(null);
		if (!result.ok) setError(result.error);
	}

	return (
		<Card padding="md">
			<div className="mb-3 flex flex-wrap items-center justify-between gap-3">
				<div className="flex items-baseline gap-2">
					<h2 className="text-sm font-medium">Values</h2>
					<span className="text-xs text-muted-foreground">
						version {shown.version} of {newest}
						{older ? " · not the newest" : ""}
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
						disabled={!dirty || busy !== null}
						loading={busy === "saving"}
					>
						Save version
					</Button>
					{/* The label names the version when it is not the newest. A bare
					    "Roll out" next to an older version on screen reads as "deploy
					    the current state", which is the wrong half of an ambiguity
					    that has a cluster on the other side of it. */}
					<Button
						icon={Rocket}
						onClick={rollout}
						disabled={busy !== null}
						loading={busy === "rolling-out"}
					>
						{older && !dirty ? `Roll out version ${shown.version}` : "Roll out"}
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
