"use client";

import { useState } from "react";
import { UserRound } from "lucide-react";
import { createUser } from "@/app/actions/redis";
import { FormField, Input } from "@/components/ui";
import { EMPTY_INSTALL } from "@/lib/secrets/install-draft";
import { effectivePrefix } from "@/lib/redis/users";
import type { Namespace } from "@/lib/api/types";
import type { RedisPreset } from "@/lib/api/redis-types";
import { CreatePanel } from "../../../_components/create-panel";
import { InstallSecretFields } from "../../../_components/install-secret-fields";
import { SecretInput } from "../../../_components/secret-input";
import { useCredentialInstall } from "../../../_components/use-credential-install";
import { ScopeFields } from "./scope-fields";

const EMPTY = { name: "", password: "", prefix: "", preset: "readwrite" as RedisPreset };

/**
 * Creating an app's Redis user, and optionally installing its credential as a
 * Secret in the same step — the same two-call submit the database users use,
 * with the same resume-on-retry when the Secret write is what failed.
 *
 * The password is required. Redis allows `nopass`, but a user that needs no
 * password is the NetworkPolicy alone standing between the keys and anything in
 * an admitted namespace.
 */
export function CreateRedisUserPanel({
	open,
	namespaces,
	namespacesError,
	onClose,
}: {
	open: boolean;
	namespaces: Namespace[];
	namespacesError: string | null;
	onClose: () => void;
}) {
	const [draft, setDraft] = useState(EMPTY);
	const secret = useCredentialInstall("user");

	function reset() {
		setDraft(EMPTY);
		secret.reset();
		onClose();
	}

	const dirty =
		JSON.stringify(draft) !== JSON.stringify(EMPTY) ||
		JSON.stringify(secret.install) !== JSON.stringify(EMPTY_INSTALL);

	return (
		<CreatePanel
			open={open}
			title="New user"
			icon={UserRound}
			description="The password never reaches Redis: the API sends its SHA-256, which is all Redis stores. The user is written to the aclfile, so it survives a restart."
			dirty={dirty}
			onClose={reset}
			onSubmit={() =>
				secret.submit({ username: draft.name, password: draft.password }, () =>
					createUser({
						name: draft.name,
						password: draft.password,
						prefix: effectivePrefix(draft.name, draft.prefix),
						preset: draft.preset,
					}),
				)
			}
		>
			<FormField label="Name" htmlFor="redis-user-name">
				<Input
					id="redis-user-name"
					value={draft.name}
					onChange={(event) => setDraft({ ...draft, name: event.target.value })}
					placeholder="orders"
					autoComplete="off"
					required
				/>
			</FormField>

			<FormField label="Password" htmlFor="redis-user-password">
				<SecretInput
					id="redis-user-password"
					value={draft.password}
					onChange={(password) => setDraft({ ...draft, password })}
					generateLabel="Generate a password"
					placeholder="At least 16 characters"
					required
				/>
			</FormField>

			<ScopeFields
				prefix={draft.prefix}
				prefixPlaceholder={effectivePrefix(draft.name, "") || "orders:"}
				preset={draft.preset}
				onPrefixChange={(prefix) => setDraft({ ...draft, prefix })}
				onPresetChange={(preset) => setDraft({ ...draft, preset })}
			/>

			<InstallSecretFields
				draft={secret.install}
				username={draft.name}
				namespaces={namespaces}
				namespacesError={namespacesError}
				onChange={secret.setInstall}
			/>
		</CreatePanel>
	);
}
