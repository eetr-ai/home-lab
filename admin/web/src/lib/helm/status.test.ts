import { describe, expect, it } from "vitest";
import { describeOutcome, isPending, stuckForMinutes, tone, jobTone, describeJob } from "./status";
import type { HelmRelease } from "@/lib/api/types";

function release(overrides: Partial<HelmRelease> = {}): HelmRelease {
	return {
		name: "whoami",
		namespace: "apps",
		revision: 2,
		status: "deployed",
		chart: "whoami",
		chartVersion: "1.1.0",
		appVersion: "1.10",
		updatedAt: "2026-08-28T12:00:00Z",
		...overrides,
	};
}

describe("isPending", () => {
	it.each(["pending-install", "pending-upgrade", "pending-rollback", "uninstalling"])(
		"%s is still running",
		(status) => expect(isPending(status)).toBe(true),
	);

	it.each(["deployed", "failed", "superseded", "uninstalled"])("%s is finished", (status) =>
		expect(isPending(status)).toBe(false),
	);
});

describe("tone", () => {
	it("reads deployed as success and failed as danger", () => {
		expect(tone("deployed")).toBe("success");
		expect(tone("failed")).toBe("danger");
	});

	it("warns while something is happening", () => {
		expect(tone("pending-upgrade")).toBe("warning");
	});

	it("leaves an old revision alone", () => {
		// superseded is what every previous revision looks like. Colouring it
		// would make an ordinary history read as a page full of problems.
		expect(tone("superseded")).toBe("muted");
		expect(tone("uninstalled")).toBe("muted");
	});
});

describe("describeOutcome", () => {
	it("is pending while the operation runs", () => {
		expect(describeOutcome(release({ status: "pending-upgrade" }), "1.1.0")).toEqual({
			state: "pending",
		});
	});

	it("succeeds when the release is deployed at the version asked for", () => {
		expect(describeOutcome(release(), "1.1.0")).toEqual({ state: "succeeded" });
	});

	it("fails when the release failed, and carries Helm's reason", () => {
		const outcome = describeOutcome(
			release({ status: "failed", description: "timed out waiting for the condition" }),
			"1.1.0",
		);
		expect(outcome).toEqual({
			state: "failed",
			reason: "timed out waiting for the condition",
		});
	});

	it("fails when the release is deployed at an older version than was asked for", () => {
		// The trap this function exists for. With rollbackOnFailure set, a failed
		// upgrade is undone and the release ends up deployed -- on the chart it
		// started from. A check that waited only for a terminal status would call
		// that a successful deploy of a version that never deployed.
		const outcome = describeOutcome(release({ chartVersion: "1.0.0" }), "1.1.0");
		expect(outcome.state).toBe("failed");
		expect(outcome).toMatchObject({ reason: expect.stringContaining("1.0.0") });
	});

	it("judges by status alone when no version was asked for", () => {
		// Looking at a release nobody just changed: there is nothing to compare
		// against, so demanding a version match would report every healthy release
		// as a failure.
		expect(describeOutcome(release({ chartVersion: "0.9.0" }))).toEqual({ state: "succeeded" });
	});

	it("falls back to the status when Helm recorded no reason", () => {
		expect(describeOutcome(release({ status: "failed" }), "1.1.0")).toEqual({
			state: "failed",
			reason: "the release is failed",
		});
	});
});

describe("stuckForMinutes", () => {
	const now = new Date("2026-08-28T12:30:00Z");

	it("reports nothing for a release that is not pending", () => {
		expect(stuckForMinutes(release(), now)).toBeNull();
	});

	it("reports nothing for a pending release that only just started", () => {
		expect(
			stuckForMinutes(
				release({ status: "pending-upgrade", updatedAt: "2026-08-28T12:25:00Z" }),
				now,
			),
		).toBeNull();
	});

	it("reports how long a pending release has been stuck", () => {
		// A pod killed mid-upgrade leaves this pending forever, and every later
		// attempt is refused for it. Surfacing it is the whole recovery path.
		expect(stuckForMinutes(release({ status: "pending-upgrade" }), now)).toBe(30);
	});

	it("reports nothing for an unparseable timestamp rather than a negative age", () => {
		expect(stuckForMinutes(release({ status: "pending-upgrade", updatedAt: "soon" }), now)).toBeNull();
	});
});

describe("jobTone", () => {
	// Pending is a pod waiting to be scheduled — the first second of every
	// operation. Colouring it warning would make every deploy start alarming.
	it("reads pending as neither progress nor trouble", () => {
		expect(jobTone("pending")).toBe("muted");
	});

	it("colours the outcomes", () => {
		expect(jobTone("running")).toBe("warning");
		expect(jobTone("succeeded")).toBe("success");
		expect(jobTone("failed")).toBe("danger");
	});
});

describe("describeJob", () => {
	// "failed" alone sends an operator to the log for something the status already
	// knows, and DeadlineExceeded means something different from a bad chart.
	it("carries the reason when Kubernetes named one", () => {
		expect(describeJob("failed", "DeadlineExceeded")).toBe("failed: DeadlineExceeded");
		expect(describeJob("failed")).toBe("failed");
	});

	it("says what the other phases mean", () => {
		expect(describeJob("pending")).toBe("waiting for a pod");
		expect(describeJob("running")).toBe("running");
		expect(describeJob("succeeded")).toBe("finished");
	});
});
