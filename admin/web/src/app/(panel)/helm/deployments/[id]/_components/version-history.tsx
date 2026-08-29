"use client";

import { useRef, useState } from "react";
import { Check, History, Minus } from "lucide-react";
import { Button, Popover, cn } from "@/components/ui";
import type { HelmDeploymentVersion } from "@/lib/api/types";
import { formatAge } from "@/lib/format/age";

/**
 * Every version ever declared, behind a button that says which one you are on.
 *
 * One control doing both jobs, deliberately. The button is the indicator — it
 * reads "Latest" while the newest version is loaded and names the version
 * otherwise — and it is also how you change which version that is, because the
 * list it opens is the way back. A separate badge and a separate "back to
 * newest" button said the same thing in three places and still left the reader
 * to connect them.
 *
 * A popover rather than a table down the page, because this is reference
 * material: it answers "what changed, and who changed it" when somebody asks,
 * and the rest of the time it pushes the thing you came for — the values — below
 * the fold. Opening it over the editor also keeps the editor in view, which
 * matters when the reason you opened it was to compare.
 *
 * Not a Directory: that primitive owns the page's main table, complete with an
 * empty state and a page-level error banner, and neither applies to a panel that
 * cannot be empty. A deployment always has at least one version, because the
 * record and its first version are written in one transaction.
 */
export function VersionHistory({
	versions,
	selected,
	now,
	onOpenVersion,
}: {
	/** Newest first, which is the order the API returns them in. */
	versions: HelmDeploymentVersion[];
	/** The version currently loaded in the editor. */
	selected: number;
	now: Date;
	onOpenVersion: (version: number) => void;
}) {
	const [open, setOpen] = useState(false);
	const trigger = useRef<HTMLButtonElement>(null);

	// versions is newest first, so the first entry is the one "latest" means.
	const newest = versions[0]?.version ?? selected;
	const older = selected !== newest;

	return (
		<>
			{/* The label carries the state and the tone reinforces it. The button
			    chrome stays the ordinary secondary pill: this is a control you press
			    to look at something, not a warning you have to clear. */}
			<Button
				ref={trigger}
				variant="secondary"
				icon={History}
				aria-expanded={open}
				aria-haspopup="dialog"
				aria-label={older ? `Version ${selected} of ${newest}. Choose a version.` : undefined}
				className={cn(older && "text-warning-fg")}
				onClick={() => setOpen((wasOpen) => !wasOpen)}
			>
				{older ? (
					`Version ${selected} of ${newest}`
				) : (
					<>
						Latest
						<span className="text-muted-foreground">({versions.length})</span>
					</>
				)}
			</Button>

			<Popover
				open={open}
				onRequestClose={() => setOpen(false)}
				anchor={trigger}
				title="Version history"
				width="lg"
			>
				<div className="border-b border-border px-4 py-3">
					<h3 className="text-sm font-medium">Version history</h3>
					<p className="mt-0.5 text-xs text-muted-foreground">
						Append-only. Choosing one loads its values into the editor; saving from there
						writes a new version rather than changing this one. The top entry is the
						latest.
					</p>
				</div>

				<ul className="divide-y divide-border">
					{versions.map((version) => (
						<li key={version.version}>
							<button
								type="button"
								onClick={() => {
									onOpenVersion(version.version);
									setOpen(false);
								}}
								className={cn(
									"flex w-full items-baseline gap-3 px-4 py-2.5 text-left text-sm outline-none ring-inset ring-brand hover:bg-surface-hover focus-visible:ring-2",
									version.version === selected && "bg-surface-hover",
								)}
							>
								<span className="w-8 shrink-0 text-right font-medium tabular-nums">
									{version.version}
								</span>
								<span className="w-24 shrink-0 truncate text-muted-foreground">
									{version.chartVersion}
								</span>
								<span className="min-w-0 flex-1 truncate text-muted-foreground" title={version.createdBy}>
									{version.source === "ci" ? "pipeline" : "panel"} &middot; {version.createdBy}
								</span>
								<Rollout version={version} now={now} />
								<span className="w-12 shrink-0 text-right text-xs text-muted-foreground">
									{formatAge(version.createdAt, now)}
								</span>
							</button>
						</li>
					))}
				</ul>
			</Popover>
		</>
	);
}

/** Whether this version ever reached the cluster, and as which Helm revision. */
function Rollout({ version, now }: { version: HelmDeploymentVersion; now: Date }) {
	if (!version.rolledOutAt) {
		return (
			<span className="inline-flex w-40 shrink-0 items-center gap-1.5 whitespace-nowrap text-xs text-muted-foreground">
				<Minus className="h-3.5 w-3.5" />
				never rolled out
			</span>
		);
	}

	return (
		<span className="inline-flex w-40 shrink-0 items-center gap-1.5 whitespace-nowrap text-xs text-muted-foreground">
			<Check className="h-3.5 w-3.5" />
			{formatAge(version.rolledOutAt, now)}
			{version.helmRevision ? ` · revision ${version.helmRevision}` : ""}
		</span>
	);
}
