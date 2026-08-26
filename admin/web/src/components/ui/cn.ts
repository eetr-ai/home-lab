/**
 * Join Tailwind class strings, dropping falsy parts. Variants in this library are
 * mutually exclusive, so no conflict-resolution (tailwind-merge) is needed — a plain
 * join keeps the output byte-identical to the hand-written class strings it replaces.
 */
export function cn(...parts: Array<string | false | null | undefined>): string {
	return parts.filter(Boolean).join(" ");
}
