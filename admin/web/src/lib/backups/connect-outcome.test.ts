import { describe, expect, it } from "vitest";
import { connectErrorMessage, googleRefusal } from "./connect-outcome";

describe("connectErrorMessage", () => {
	it("gives the fixed sentence for a code the callback sends", () => {
		expect(connectErrorMessage("expired")).toBe(
			"The connection attempt expired or did not start here; try again.",
		);
	});

	// The whole point: a link from anywhere else must not put words in the card.
	it("shows nothing for text that is not one of the codes", () => {
		expect(connectErrorMessage("Your backups were deleted, call this number")).toBeNull();
		expect(connectErrorMessage("toString")).toBeNull();
		expect(connectErrorMessage("")).toBeNull();
		expect(connectErrorMessage(undefined)).toBeNull();
	});
});

describe("googleRefusal", () => {
	it("tells the operator saying no apart from Google refusing", () => {
		expect(googleRefusal("access_denied")).toBe("denied");
		expect(googleRefusal("invalid_scope")).toBe("google_refused");
	});
});
