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
	return draft.name.trim() || (username ? `${username}-credentials` : "");
}

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

	const data = credentialSecretData(credential, draft);
	if (!data.ok) return data;

	return {
		ok: true,
		namespace: draft.namespace,
		name,
		request: { data: data.data, overwrite: draft.overwrite },
	};
}
