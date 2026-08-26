/** Counts a noun, so the ternary is not repeated at every call site. */
export function plural(count: number, noun: string): string {
	return `${count} ${noun}${count === 1 ? "" : "s"}`;
}
