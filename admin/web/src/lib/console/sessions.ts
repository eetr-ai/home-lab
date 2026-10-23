import { DEFAULT_SESSION, SESSION_NAME } from "./session-name";

/**
 * The console's open tabs.
 *
 * A tab is a view onto a tmux session in the console pod, and the session outlives
 * the tab: closing one detaches, it does not end anything. So this list is only
 * which sessions this browser is looking at. It is kept in localStorage as a
 * convenience; a name that comes back after its session ended simply starts a
 * fresh one under the same name, which is what reopening it would do anyway.
 */

export interface SessionsState {
	sessions: string[];
	active: string | null;
}

export type SessionsAction =
	| { type: "open" }
	| { type: "select"; name: string }
	| { type: "close"; name: string };

export const STORAGE_KEY = "console-sessions";

export function sessionsReducer(state: SessionsState, action: SessionsAction): SessionsState {
	switch (action.type) {
		case "open": {
			const name = nextName(state.sessions);
			return { sessions: [...state.sessions, name], active: name };
		}
		case "select":
			return state.sessions.includes(action.name) ? { ...state, active: action.name } : state;
		case "close": {
			const index = state.sessions.indexOf(action.name);
			if (index === -1) return state;
			const sessions = state.sessions.filter((name) => name !== action.name);
			// Closing the tab you are on lands on its neighbour, the way browser
			// tabs do; closing another one leaves you where you were.
			const active =
				state.active !== action.name
					? state.active
					: (sessions[Math.min(index, sessions.length - 1)] ?? null);
			return { sessions, active };
		}
	}
}

/** `main` first, then `shell-2`, `shell-3`… — the lowest number not in use. */
export function nextName(taken: string[]): string {
	if (!taken.includes(DEFAULT_SESSION)) return DEFAULT_SESSION;
	for (let n = 2; ; n++) {
		const name = `shell-${n}`;
		if (!taken.includes(name)) return name;
	}
}

/**
 * The state to start from, given what localStorage held. Anything unreadable,
 * and any name the gate would refuse, is dropped; an empty result opens `main`,
 * because a console page with no terminal on it is not what anyone came for.
 */
export function restoreSessions(stored: string | null): SessionsState {
	let parsed: unknown = null;
	try {
		parsed = stored ? JSON.parse(stored) : null;
	} catch {
		parsed = null;
	}
	const names = Array.isArray(parsed)
		? [...new Set(parsed.filter((n): n is string => typeof n === "string" && SESSION_NAME.test(n)))]
		: [];
	const sessions = names.length > 0 ? names : [DEFAULT_SESSION];
	return { sessions, active: sessions[0] };
}
