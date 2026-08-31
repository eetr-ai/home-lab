"use client";

import { useRef, useState } from "react";
import { putSecret } from "@/app/actions/kube";
import { EMPTY_INSTALL, planInstall, type InstallDraft } from "@/lib/secrets/install-draft";
import type { Credential } from "@/lib/secrets/db-secret";
import type { ActionResult } from "@/lib/api/result";

/**
 * Creating a database credential and installing it as a Secret, as one submit.
 *
 * Two calls behind one button, and the order matters: the role is created first,
 * because a Secret holding a password no account has is worse than no Secret.
 *
 * Which leaves the case this hook exists for. If the role is created and the
 * Secret write then fails, pressing the button again must not try to create the
 * role a second time — that answers 409 and the operator is left staring at a
 * conflict about something that already worked. So the first success is
 * remembered, and a retry resumes at the step that failed. The password is still
 * in the form, which is what makes resuming possible at all: nothing stores it,
 * and closing the panel is what throws it away.
 */
export function useCredentialInstall(noun: string) {
	const [install, setInstall] = useState<InstallDraft>(EMPTY_INSTALL);
	const created = useRef(false);

	function reset() {
		setInstall(EMPTY_INSTALL);
		created.current = false;
	}

	async function submit(
		credential: Credential,
		create: () => Promise<ActionResult<unknown>>,
	): Promise<ActionResult<unknown>> {
		// Checked before anything is created: a layout that cannot produce a Secret
		// is a typo in this form, and finding it after the role exists would mean
		// the operator has to think about a half-finished state for no reason.
		const plan = planInstall(install, credential);
		if (plan && !plan.ok) return { ok: false, error: plan.error };

		if (!created.current) {
			const result = await create();
			if (!result.ok) return result;
			created.current = true;
		}

		if (!plan) return { ok: true, data: undefined };

		const written = await putSecret(plan.namespace, plan.name, plan.request);
		if (!written.ok) {
			return {
				ok: false,
				error: `The ${noun} was created, but the Secret was not written: ${written.error}`,
			};
		}
		return written;
	}

	return { install, setInstall, submit, reset };
}
