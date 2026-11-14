import type { RedisPreset, RedisUser } from "@/lib/api/redis-types";

/**
 * The rules of the Redis users form that are worth testing without React.
 */

/** What each preset means, in the words the form shows. */
export const PRESETS: { value: RedisPreset; label: string; hint: string }[] = [
	{
		value: "readwrite",
		label: "Read and write",
		hint: "Every command except the dangerous ones (FLUSHALL, KEYS, CONFIG…), on its own keys only. No SCAN: it would list other apps' key names.",
	},
	{
		value: "readonly",
		label: "Read only",
		hint: "Reads on its own keys, and nothing that changes them. No SCAN, for the same reason.",
	},
];

/**
 * The prefix a new user will get.
 *
 * Left empty, the prefix is the user's name and a colon — `orders:` for
 * `orders` — which is the convention most Redis clients already namespace keys
 * with. The API refuses an empty prefix outright (a user confined to "" is
 * confined to nothing), so filling it in here is what lets the field be
 * optional.
 */
export function effectivePrefix(name: string, prefix: string): string {
	const trimmed = prefix.trim();
	if (trimmed) return trimmed;
	return name.trim() ? `${name.trim()}:` : "";
}

/** How a user's permissions read in the list. */
export function describeUser(user: RedisUser): string {
	if (user.name === "default") return "Everything — the platform's own credential";
	// Checked before the rules: a user with no password is not editable here
	// whatever its rules, and "custom rules" would send someone looking for the
	// wrong thing.
	if (!user.hasPassword) return "No password, set outside the panel";
	if (!user.editable) return "Custom rules, set outside the panel";
	const preset = PRESETS.find((one) => one.value === user.preset);
	return `${preset?.label ?? user.preset} on ${user.prefix}*`;
}
