"use client";

import { FormField, Input, Select } from "@/components/ui";
import { PRESETS } from "@/lib/redis/users";
import type { RedisPreset } from "@/lib/api/redis-types";

/**
 * What a Redis user may reach and do: a key prefix and a preset.
 *
 * Shared by the create and edit panels because it is the same two questions in
 * both. The prefix applies to pub/sub channels as well as keys — one namespace
 * for everything the app touches — and the hint says so, because a channel
 * outside it fails at SUBSCRIBE time in a workload, a long way from this form.
 */
export function ScopeFields({
	prefix,
	prefixPlaceholder,
	preset,
	onPrefixChange,
	onPresetChange,
}: {
	prefix: string;
	/** What an empty prefix becomes, shown rather than explained. */
	prefixPlaceholder: string;
	preset: RedisPreset;
	onPrefixChange: (prefix: string) => void;
	onPresetChange: (preset: RedisPreset) => void;
}) {
	const chosen = PRESETS.find((one) => one.value === preset);

	return (
		<>
			<FormField label="Key prefix" htmlFor="redis-user-prefix">
				<Input
					id="redis-user-prefix"
					className="font-mono"
					value={prefix}
					onChange={(event) => onPrefixChange(event.target.value)}
					placeholder={prefixPlaceholder}
					autoComplete="off"
				/>
				<p className="mt-1 text-xs text-muted-foreground">
					The user reaches only keys and pub/sub channels starting with this. A prefix, not a pattern: no * or ?.
				</p>
			</FormField>

			<FormField label="Permissions" htmlFor="redis-user-preset">
				<Select
					id="redis-user-preset"
					className="w-full"
					value={preset}
					onChange={(event) => onPresetChange(event.target.value as RedisPreset)}
				>
					{PRESETS.map((one) => (
						<option key={one.value} value={one.value}>
							{one.label}
						</option>
					))}
				</Select>
				{chosen ? <p className="mt-1 text-xs text-muted-foreground">{chosen.hint}</p> : null}
			</FormField>
		</>
	);
}
