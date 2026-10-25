import { describe, expect, it } from "vitest";
import { emptyForm, formFrom, toRequest, type ScheduleForm } from "./form";

function filled(change: Partial<ScheduleForm> = {}): ScheduleForm {
	return {
		...emptyForm("UTC"),
		name: "nightly",
		databases: ["postgres/app", "mongo/events"],
		folder: { id: "1AbCdEfGhIjKlMnOp", name: "Lab backups" },
		...change,
	};
}

describe("toRequest", () => {
	it("builds the request the API takes", () => {
		const result = toRequest(filled({ keep: "14", cron: " 0 3 * * * " }));
		expect(result).toEqual({
			ok: true,
			request: {
				name: "nightly",
				cron: "0 3 * * *",
				timeZone: "UTC",
				databases: [
					{ engine: "postgres", name: "app" },
					{ engine: "mongo", name: "events" },
				],
				folder: { id: "1AbCdEfGhIjKlMnOp", name: "Lab backups" },
				keep: 14,
				suspended: false,
			},
		});
	});

	it("leaves an empty zone out, so the cluster's applies", () => {
		const result = toRequest(filled({ timeZone: " " }));
		expect(result.ok && result.request.timeZone).toBeUndefined();
	});

	it.each([
		["an uppercase name", { name: "Nightly" }, "name"],
		["a name with a trailing dash", { name: "nightly-" }, "name"],
		["no databases", { databases: [] }, "database"],
		["no folder", { folder: null }, "folder"],
		["keeping none", { keep: "0" }, "Keep"],
		["keeping a fraction", { keep: "2.5" }, "Keep"],
		["keeping too many", { keep: "101" }, "Keep"],
		["no schedule", { cron: " " }, "when"],
	])("refuses %s", (_, change, word) => {
		const result = toRequest(filled(change as Partial<ScheduleForm>));
		expect(result.ok).toBe(false);
		expect(!result.ok && result.error).toContain(word);
	});
});

describe("formFrom", () => {
	it("round-trips a schedule through the form", () => {
		const schedule = {
			name: "nightly",
			cron: "0 3 * * *",
			timeZone: "UTC",
			databases: [{ engine: "postgres" as const, name: "app" }],
			folder: { id: "1AbCdEfGhIjKlMnOp", name: "Lab backups" },
			keep: 7,
			suspended: true,
		};
		expect(toRequest(formFrom(schedule))).toEqual({ ok: true, request: schedule });
	});
});
