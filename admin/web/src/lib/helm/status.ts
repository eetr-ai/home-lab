import type { HelmJobPhase, HelmRelease } from "@/lib/api/types";

/**
 * Reading a Helm release's status.
 *
 * Kept out of the components because the rules are easy to state and easy to get
 * wrong, which is exactly what docs/contributing/testing.md says to extract.
 * `describeOutcome` in particular is not a formatter: it is the rule a pipeline
 * would use, written once so the panel and the documentation agree.
 */

/** The states Helm reports while something is still happening. */
const PENDING = new Set([
	"pending-install",
	"pending-upgrade",
	"pending-rollback",
	"uninstalling",
]);

/** Whether an operation on this release is still running. */
export function isPending(status: string): boolean {
	return PENDING.has(status);
}

export type Tone = "success" | "danger" | "warning" | "muted";

/** How a status should read at a glance. */
export function tone(status: string): Tone {
	if (status === "deployed") return "success";
	if (status === "failed") return "danger";
	if (isPending(status)) return "warning";
	// superseded and uninstalled are neither good nor bad — they are what an old
	// revision looks like, and colouring them would make history alarming.
	return "muted";
}

export type Outcome =
	| { state: "pending" }
	| { state: "succeeded" }
	| { state: "failed"; reason: string };

/**
 * Whether a release ended up where it was asked to go.
 *
 * The rule that matters, and the one that is easy to get wrong: **a terminal
 * status is not success.** With rollbackOnFailure set, a failed upgrade is undone
 * and the release lands `deployed` on a new revision — of an *earlier* chart. A
 * check that only waited for the status to stop being pending would read that as
 * a successful deploy of a version that was never deployed.
 *
 * So success is `deployed` AND the chart version being the one that was asked
 * for. This is the same rule the documented pipeline check uses; if one changes,
 * both do.
 *
 * With no requested version — looking at a release nobody just changed — there is
 * nothing to compare against and the status alone is the answer.
 */
export function describeOutcome(release: HelmRelease, requestedVersion?: string): Outcome {
	if (isPending(release.status)) return { state: "pending" };

	if (release.status !== "deployed") {
		return { state: "failed", reason: release.description || `the release is ${release.status}` };
	}

	if (requestedVersion && release.chartVersion !== requestedVersion) {
		return {
			state: "failed",
			reason:
				`the release is deployed at ${release.chartVersion} rather than ` +
				`${requestedVersion} — it was most likely rolled back after failing`,
		};
	}

	return { state: "succeeded" };
}

/**
 * How long a release has been stuck, in whole minutes, or null when it is not.
 *
 * A pod killed mid-upgrade leaves the release pending forever: Helm's storage has
 * no timeout, and every later attempt is refused for it. Nothing here recovers
 * that automatically — two replicas plus a guess about whether somebody else's
 * operation is dead is how a release gets corrupted — so the panel surfaces it
 * and a human decides.
 */
export function stuckForMinutes(release: HelmRelease, now: Date, threshold = 15): number | null {
	if (!isPending(release.status)) return null;

	const since = Date.parse(release.updatedAt);
	if (Number.isNaN(since)) return null;

	const minutes = Math.floor((now.getTime() - since) / 60_000);
	return minutes >= threshold ? minutes : null;
}

/** How a job's phase should read at a glance. */
export function jobTone(phase: HelmJobPhase): Tone {
	switch (phase) {
		case "succeeded":
			return "success";
		case "failed":
			return "danger";
		case "running":
			return "warning";
		// Pending is a pod waiting to be scheduled, which is neither progress nor
		// trouble. Colouring it warning would make every operation start alarming.
		default:
			return "muted";
	}
}

/**
 * What a job is doing, in words.
 *
 * The reason is worth surfacing where Kubernetes gave one, because "failed" alone
 * sends an operator to the log for something the status already knows —
 * DeadlineExceeded in particular means the operation ran out of time rather than
 * that the chart was wrong, and those lead to different next steps.
 */
export function describeJob(phase: HelmJobPhase, reason?: string): string {
	switch (phase) {
		case "pending":
			return "waiting for a pod";
		case "running":
			return "running";
		case "succeeded":
			return "finished";
		case "failed":
			return reason ? `failed: ${reason}` : "failed";
	}
}
