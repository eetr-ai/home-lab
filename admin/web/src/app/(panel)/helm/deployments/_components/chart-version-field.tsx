"use client";

import { RefreshCw } from "lucide-react";
import { IconButton, Input, Select } from "@/components/ui";
import { versionField } from "./chart-version-options";
import { useChartVersions } from "./use-chart-versions";

/**
 * The chart version, as a list to pick from wherever the registry will say what
 * it publishes.
 *
 * A picker when the registry answered, a text field when it did not — and a text
 * field is also what an upgrade starts as, since nothing has asked a registry
 * anything yet. An unreachable registry is a reason to type the version yourself,
 * not a reason to be unable to declare or upgrade anything, so the input is the
 * fallback rather than the thing that gets disabled.
 *
 * The refresh button is the whole control on the upgrade side, where `follow` is
 * off: there, the reference has not changed and nothing should reach a registry
 * just because a page was opened. Pressing it is the operator asking, and it is
 * also how a version published since the page loaded appears without a reload.
 *
 * The current value is always among the options, even when the registry does not
 * offer it. A `select` whose value is not one of its options renders as the first
 * one, so a chart that has since dropped the version a release is running on
 * would show — and then silently save — a different version than the one it was
 * asked about.
 */
export function ChartVersionField({
	id,
	reference,
	value,
	onChange,
	follow = false,
	dense = false,
}: {
	id: string;
	/** The chart the versions belong to. */
	reference: string;
	value: string;
	onChange: (version: string) => void;
	/** Look the reference up as it is typed, rather than only on refresh. */
	follow?: boolean;
	/** Toolbar sizing rather than form sizing. */
	dense?: boolean;
}) {
	const versions = useChartVersions(reference, follow);

	const { picker, options } = versionField(
		versions.offered.map((one) => one.version),
		value,
	);
	const appVersions = new Map(versions.offered.map((one) => [one.version, one.appVersion]));

	return (
		<div className={dense ? "flex flex-col gap-1" : undefined}>
			<div className="flex items-center gap-1">
				{picker ? (
					<Select
						id={id}
						value={value}
						onChange={(event) => onChange(event.target.value)}
						className={dense ? "w-44 py-1 text-xs" : "w-full"}
						required
					>
						{value === "" ? <option value="">Choose a version</option> : null}
						{options.map((version) => {
							const appVersion = appVersions.get(version);
							return (
								<option key={version} value={version}>
									{appVersion ? `${version} (app ${appVersion})` : version}
								</option>
							);
						})}
					</Select>
				) : (
					<Input
						id={id}
						value={value}
						onChange={(event) => onChange(event.target.value)}
						placeholder="6.9.2"
						autoComplete="off"
						spellCheck={false}
						className={dense ? "w-28 px-2 py-1 text-xs" : undefined}
						required
					/>
				)}
				<IconButton
					// This renders inside the declare form, and a button with no type
					// is a submit button: without this, asking what versions exist
					// would declare the deployment.
					type="button"
					aria-label="List the versions this chart publishes"
					title="List the versions this chart publishes"
					onClick={versions.refresh}
					loading={versions.loading}
					// Not merely unhelpful without a reference — the API answers 400
					// for a half-typed one, so the button would report a failure the
					// operator caused by not having finished typing.
					disabled={!versions.askable}
				>
					<RefreshCw className="h-4 w-4" />
				</IconButton>
			</div>
			<Hint dense={dense}>{versions.hint}</Hint>
		</div>
	);
}

/** A line of guidance under the control. Nothing renders for an empty one. */
function Hint({ children, dense }: { children?: React.ReactNode; dense: boolean }) {
	if (!children) return null;
	return <p className={dense ? "text-xs text-muted-foreground" : "mt-1 text-xs text-muted-foreground"}>{children}</p>;
}
