/**
 * Which tmux session a socket attaches to.
 *
 * The name crosses three hands — the browser, this gate, and `attach.sh` in the
 * console image — and ends up as an argument to tmux. The image checks it again,
 * with the same pattern, because it is the last one to see it. This is the first,
 * so a bad name is a 400 here rather than a shell that opens and dies.
 */

/** Letters, digits, `-` and `_`; one to 32 of them. Mirrors admin/console/attach.sh. */
export const SESSION_NAME = /^[A-Za-z0-9_-]{1,32}$/;

/** The session a request asks for when it names none. */
export const DEFAULT_SESSION = "main";

/**
 * The session named by `raw`, or null when it is not one we would hand to tmux.
 * Absent and empty both mean the default; anything else must match exactly.
 */
export function sessionName(raw: string | null | undefined): string | null {
	if (raw === null || raw === undefined || raw === "") return DEFAULT_SESSION;
	return SESSION_NAME.test(raw) ? raw : null;
}
