import { describe, expect, it } from "vitest";
import { namespaceScopeFor } from "./scoped-routes";

describe("namespaceScopeFor", () => {
	it.each([
		["/kubernetes/workloads", "optional"],
		["/kubernetes/pods", "optional"],
		["/kubernetes/events", "optional"],
		["/helm/dashboard", "optional"],
		["/helm/deployments", "optional"],
		["/kubernetes/secrets", "required"],
	] as const)("scopes %s as %s", (path, use) => {
		expect(namespaceScopeFor(path)).toBe(use);
	});

	it("ignores a trailing slash", () => {
		expect(namespaceScopeFor("/kubernetes/pods/")).toBe("optional");
	});

	// A detail page sits under its list's path and is about one object in one
	// namespace. Inheriting the list's scope would put a switcher above it that
	// looks like it applies.
	it.each([
		"/kubernetes/namespaces",
		"/kubernetes/nodes",
		"/kubernetes/storage",
		"/kubernetes/workloads/deployment/admin-api",
		"/helm/dashboard/admin/home-lab-admin",
		"/helm/deployments/070a9a71",
		"/postgres",
		"/",
	])("does not scope %s", (path) => {
		expect(namespaceScopeFor(path)).toBe("none");
	});
});
