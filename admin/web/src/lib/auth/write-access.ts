/**
 * Who may perform a mutating operation.
 *
 * eetr-auth issues no role or group claim, so the access boundary is membership:
 * it refuses `/authorize` unless the user is granted this client's environment,
 * and anyone holding a token for this client is an operator here. That is the
 * right default for a single-operator panel, and it is the default below.
 *
 * `ADMIN_WRITE_EMAILS` narrows it. Setting it makes everyone else read-only,
 * which is the only way to hand someone the panel without also handing them
 * `DROP DATABASE`. Pure, so the rule can be tested without a session.
 */

/** Parse the comma-separated allowlist. An unset or blank value permits everyone. */
export function writeAllowlist(raw: string | undefined): string[] {
	return (raw ?? "")
		.split(",")
		.map((entry) => entry.trim().toLowerCase())
		.filter(Boolean);
}

/**
 * Whether `email` may write, given the allowlist.
 *
 * An empty allowlist permits any signed-in operator. A non-empty one permits only
 * the addresses in it — and a caller with no email at all is refused rather than
 * admitted, because an allowlist that cannot identify you has not allowed you.
 */
export function permitsWrite(allowlist: string[], email: string | null | undefined): boolean {
	if (allowlist.length === 0) return true;
	if (!email) return false;
	return allowlist.includes(email.trim().toLowerCase());
}
