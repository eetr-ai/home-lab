import { credentialSecretData, DEFAULT_LAYOUT, type Credential, type SecretLayout } from "./db-secret";
import type { PutSecret } from "@/lib/api/types";

/**
 * The "also install this as a Secret" half of a create-credential form.
 *
 * Kept out of the component because it is the part with rules: which fields are
 * required once the section is switched on, what the Secret is called when the
 * operator has not said, and how the credential becomes a payload. The component
 * renders it and owns nothing else.
 */
export interface InstallDraft extends SecretLayout {
	/** Off by default: creating a role is the request, and this is an extra. */
	enabled: boolean;
	namespace: string;
	/** Empty means "name it after the credential" — see secretName. */
	name: string;
	overwrite: boolean;
}

export const EMPTY_INSTALL: InstallDraft = {
	...DEFAULT_LAYOUT,
	enabled: false,
	namespace: "",
	name: "",
	overwrite: false,
};

export type InstallPlan =
	| { ok: true; namespace: string; name: string; request: PutSecret }
	| { ok: false; error: string };

/**
 * What the Secret is called when the operator has not said.
 *
 * `<credential>-credentials` rather than the bare name: a Secret sitting beside
 * a release named after the same thing is easier to mistake for the release's
 * own than one that says what it holds.
 */
export function secretName(draft: InstallDraft, username: string): string {
	return draft.name.trim() || defaultSecretName(username);
}

/**
 * The derived name, made legal.
 *
 * `analytics_app` is an ordinary PostgreSQL role and an illegal Secret name, so
 * the obvious derivation produces something the API refuses — after the role has
 * been created. Underscores become hyphens, capitals come down, and anything else
 * is dropped; a name the operator types is left exactly as typed and checked.
 */
function defaultSecretName(username: string): string {
	const stem = username
		.trim()
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, "-")
		.replace(/^-+|-+$/g, "");
	return stem ? `${stem}-credentials` : "";
}

/**
 * The name rule the API applies, repeated here so a typo is caught before the
 * credential exists.
 *
 * A DNS-1123 *label*, not the subdomain Kubernetes would allow: the API is
 * deliberately stricter — see validateSecretName — and a client that were more
 * permissive would let `analytics credentials` through, create the role, and then
 * fail the write with the operator holding half a result.
 */
const NAME_PATTERN = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/;
const MAX_NAME_LENGTH = 63;

/**
 * Turns the draft and the credential into the call to make, or into the reason
 * there is nothing to call with.
 *
 * Returns null when the section is switched off, which is the common case and is
 * not an error — the caller creates the role and stops there.
 */
export function planInstall(draft: InstallDraft, credential: Credential): InstallPlan | null {
	if (!draft.enabled) return null;

	if (!draft.namespace) return { ok: false, error: "Choose a namespace for the Secret." };

	const name = secretName(draft, credential.username);
	if (!name) return { ok: false, error: "Name the Secret." };
	if (name.length > MAX_NAME_LENGTH || !NAME_PATTERN.test(name)) {
		return {
			ok: false,
			error: `"${name}" is not a valid Secret name: lowercase letters, digits and ` +
				`hyphens, starting and ending with a letter or digit, at most ` +
				`${MAX_NAME_LENGTH} characters.`,
		};
	}

	const data = credentialSecretData(credential, draft);
	if (!data.ok) return data;

	return {
		ok: true,
		namespace: draft.namespace,
		name,
		request: { data: data.data, overwrite: draft.overwrite },
	};
}
