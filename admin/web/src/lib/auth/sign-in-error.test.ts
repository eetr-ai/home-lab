import { describe, expect, it } from "vitest";
import { signInErrorMessage } from "./sign-in-error";

describe("signInErrorMessage", () => {
	it("reports nothing when no error was passed", () => {
		expect(signInErrorMessage(undefined)).toBeNull();
		expect(signInErrorMessage("")).toBeNull();
	});

	// The one failure an operator will actually hit, and the one whose fix is not
	// in this repository at all.
	it("explains the environment grant behind AccessDenied", () => {
		expect(signInErrorMessage("AccessDenied")).toMatch(/environment/);
	});

	it("falls back for a code it does not know", () => {
		expect(signInErrorMessage("SomethingNew")).toMatch(/Sign-in failed/);
	});
});
