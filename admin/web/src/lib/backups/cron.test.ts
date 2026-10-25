import { describe, expect, it } from "vitest";
import { CUSTOM_CRON, describeCron, presetFor } from "./cron";

describe("presetFor", () => {
	it("recognises a preset however it is spaced", () => {
		expect(presetFor("0  3 * *  * ")).toBe("0 3 * * *");
	});

	it("calls anything else custom", () => {
		expect(presetFor("15 2 * * 1-5")).toBe(CUSTOM_CRON);
		expect(presetFor("")).toBe(CUSTOM_CRON);
	});
});

describe("describeCron", () => {
	it("reads a preset in words, with its zone", () => {
		expect(describeCron("0 3 * * *", "America/Los_Angeles")).toBe(
			"Every night at 03:00 (America/Los_Angeles)",
		);
	});

	// No zone means the controller's, and that should be said rather than implied.
	it("says when the cluster's clock is the one that counts", () => {
		expect(describeCron("15 2 * * 1-5")).toBe("On the cron line 15 2 * * 1-5 (cluster time)");
	});
});
