import { describe, expect, it } from "vitest";
import { nextName, restoreSessions, sessionsReducer, type SessionsState } from "./sessions";

describe("nextName", () => {
	it("starts with main", () => {
		expect(nextName([])).toBe("main");
	});

	it("numbers the rest from two", () => {
		expect(nextName(["main"])).toBe("shell-2");
		expect(nextName(["main", "shell-2"])).toBe("shell-3");
	});

	it("reuses the lowest number given back", () => {
		expect(nextName(["main", "shell-3"])).toBe("shell-2");
	});

	it("gives main back once it is closed", () => {
		expect(nextName(["shell-2"])).toBe("main");
	});
});

describe("sessionsReducer", () => {
	const three: SessionsState = { sessions: ["main", "shell-2", "shell-3"], active: "shell-2" };

	it("opens a new session and switches to it", () => {
		expect(sessionsReducer(three, { type: "open" })).toEqual({
			sessions: ["main", "shell-2", "shell-3", "shell-4"],
			active: "shell-4",
		});
	});

	it("selects an open session", () => {
		expect(sessionsReducer(three, { type: "select", name: "main" }).active).toBe("main");
	});

	it("ignores selecting a session that is not open", () => {
		expect(sessionsReducer(three, { type: "select", name: "nope" })).toBe(three);
	});

	it("closing the active tab lands on the one that took its place", () => {
		expect(sessionsReducer(three, { type: "close", name: "shell-2" })).toEqual({
			sessions: ["main", "shell-3"],
			active: "shell-3",
		});
	});

	it("closing the last tab while on it lands on the one before", () => {
		const onLast = { ...three, active: "shell-3" };
		expect(sessionsReducer(onLast, { type: "close", name: "shell-3" }).active).toBe("shell-2");
	});

	it("closing another tab stays where you are", () => {
		expect(sessionsReducer(three, { type: "close", name: "main" }).active).toBe("shell-2");
	});

	it("closing the only tab leaves nothing active", () => {
		expect(
			sessionsReducer({ sessions: ["main"], active: "main" }, { type: "close", name: "main" }),
		).toEqual({ sessions: [], active: null });
	});

	it("ignores closing a session that is not open", () => {
		expect(sessionsReducer(three, { type: "close", name: "nope" })).toBe(three);
	});
});

describe("restoreSessions", () => {
	it("opens main when nothing was stored", () => {
		expect(restoreSessions(null)).toEqual({ sessions: ["main"], active: "main" });
	});

	it("restores what was stored, first one active", () => {
		expect(restoreSessions('["deploys","main"]')).toEqual({
			sessions: ["deploys", "main"],
			active: "deploys",
		});
	});

	it("drops names the gate would refuse, and duplicates", () => {
		expect(restoreSessions('["ok","x;y",3,"ok",""]').sessions).toEqual(["ok"]);
	});

	it("treats anything unreadable as nothing stored", () => {
		expect(restoreSessions("{not json").sessions).toEqual(["main"]);
		expect(restoreSessions('{"a":1}').sessions).toEqual(["main"]);
		expect(restoreSessions("[]").sessions).toEqual(["main"]);
	});
});
