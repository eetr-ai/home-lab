import { describe, expect, it } from "vitest";
import { enrolmentView } from "./enrolment";
import type { Namespace } from "@/lib/api/types";

function namespace(fields: Partial<Namespace>): Namespace {
	return {
		name: "octo",
		status: "Active",
		createdAt: "2026-08-31T00:00:00Z",
		protected: false,
		helmManaged: true,
		...fields,
	};
}

describe("enrolmentView", () => {
	it("offers setting up a namespace that is not", () => {
		expect(enrolmentView(namespace({ helmEnrolment: "missing" }))?.action).toEqual({
			label: "Set up",
			danger: false,
		});
	});

	// The state this exists for. Bindings an older chart left pointing elsewhere
	// keep failing deploys and look fine from anywhere else.
	it("offers repairing a namespace set up wrongly", () => {
		const view = enrolmentView(namespace({ helmEnrolment: "wrong" }));
		expect(view?.action?.label).toBe("Repair");
		expect(view?.tone).toBe("warn");
	});

	it("offers repairing a half-enrolled namespace", () => {
		expect(enrolmentView(namespace({ helmEnrolment: "partial" }))?.action?.label).toBe("Repair");
	});

	it("offers revoking one that is set up", () => {
		expect(enrolmentView(namespace({ helmEnrolment: "enrolled" }))?.action).toEqual({
			label: "Revoke",
			danger: true,
		});
	});

	// Pressing a button here would be acting on an answer the panel does not have.
	it("offers nothing when the bindings could not be read", () => {
		const view = enrolmentView(namespace({ helmEnrolment: "unknown" }));
		expect(view?.label).toBe("unknown");
		expect(view?.action).toBeNull();
	});

	// The panel's own namespace is protected from deletion and deployable, and it
	// is the namespace most likely to be a Helm target — hiding its enrolment
	// because it carries a delete refusal was wrong, and looked fine until the
	// table was rendered.
	it("shows enrolment for a protected namespace that is a Helm target", () => {
		const view = enrolmentView(namespace({ protected: true, helmEnrolment: "enrolled" }));
		expect(view?.label).toBe("set up");
	});

	// A namespace that has not asked to be a Helm target is not one that is set up
	// wrongly, and the panel must not invite an operator to "fix" it.
	it("shows nothing for a namespace that is not a Helm candidate", () => {
		expect(enrolmentView(namespace({}))).toBeNull();
	});
});
