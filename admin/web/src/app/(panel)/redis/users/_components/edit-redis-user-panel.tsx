"use client";

import { useState } from "react";
import { UserRound } from "lucide-react";
import { updateUser } from "@/app/actions/redis";
import { FormField } from "@/components/ui";
import type { RedisPreset, RedisUser } from "@/lib/api/redis-types";
import { CreatePanel } from "../../../_components/create-panel";
import { SecretInput } from "../../../_components/secret-input";
import { ScopeFields } from "./scope-fields";

/**
 * Editing a user's prefix and preset, and optionally its password.
 *
 * Only offered for users whose rules are exactly a prefix and a preset, so the
 * form can show everything the user has and saving it cannot throw a rule away.
 * The whole state is sent; an empty password keeps the current one.
 */
export function EditRedisUserPanel({ user, onClose }: { user: RedisUser | null; onClose: () => void }) {
	// Keyed by user upstream, so a new instance starts from that user's state.
	const initial = {
		prefix: user?.prefix ?? "",
		preset: (user?.preset === "readonly" ? "readonly" : "readwrite") as RedisPreset,
	};
	const [scope, setScope] = useState(initial);
	const [password, setPassword] = useState("");

	const dirty = password !== "" || scope.prefix !== initial.prefix || scope.preset !== initial.preset;

	return (
		<CreatePanel
			open={user !== null}
			title={user ? `Edit ${user.name}` : "Edit user"}
			icon={UserRound}
			submitLabel="Save"
			description="Connections already open keep the permissions they authenticated with until they reconnect. Leave the password empty to keep the current one."
			dirty={dirty}
			onClose={() => {
				setScope(initial);
				setPassword("");
				onClose();
			}}
			onSubmit={async () => {
				if (!user) return { ok: false as const, error: "no user selected" };
				return updateUser(user.name, {
					prefix: scope.prefix.trim(),
					preset: scope.preset,
					...(password ? { password } : {}),
				});
			}}
		>
			<ScopeFields
				prefix={scope.prefix}
				prefixPlaceholder={initial.prefix}
				preset={scope.preset}
				onPrefixChange={(prefix) => setScope({ ...scope, prefix })}
				onPresetChange={(preset) => setScope({ ...scope, preset })}
			/>

			<FormField label="New password" htmlFor="redis-user-new-password">
				<SecretInput
					id="redis-user-new-password"
					value={password}
					onChange={setPassword}
					generateLabel="Generate a password"
					placeholder="Leave empty to keep the current one"
				/>
			</FormField>
		</CreatePanel>
	);
}
