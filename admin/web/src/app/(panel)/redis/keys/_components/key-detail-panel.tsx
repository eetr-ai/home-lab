"use client";

import { KeyRound } from "lucide-react";
import { Banner, SidePanel, Spinner } from "@/components/ui";
import { formatBytes } from "@/lib/format/bytes";
import { displayText, formatTtl } from "@/lib/redis/text";
import type { ActionResult } from "@/lib/api/result";
import type { RedisKeyDetail, RedisText } from "@/lib/api/redis-types";
import { KeyValue } from "./key-value";

/**
 * One key, opened.
 *
 * A SidePanel rather than a Popover: a hash or a stream is a page of its own to
 * read, and while it is open the key is the subject. There is no form, so no
 * unsaved-changes guard — and no delete either, which stays in the row, inline,
 * where every other delete in the panel is.
 */
export function KeyDetailPanel({
	target,
	detail,
	onClose,
}: {
	target: RedisText | null;
	/** Null while the read is in flight. */
	detail: ActionResult<RedisKeyDetail> | null;
	onClose: () => void;
}) {
	return (
		<SidePanel
			open={target !== null}
			onRequestClose={onClose}
			title={target ? displayText(target) : "Key"}
			icon={KeyRound}
			width="lg"
		>
			{detail === null ? (
				<Spinner />
			) : !detail.ok ? (
				<Banner variant="error" message={detail.error} />
			) : (
				<Body detail={detail.data} />
			)}
		</SidePanel>
	);
}

function Body({ detail }: { detail: RedisKeyDetail }) {
	const unit = detail.type === "string" ? "bytes" : "items";

	return (
		<div className="space-y-4">
			<dl className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
				<Fact label="Type" value={detail.type} />
				<Fact label="Expires in" value={formatTtl(detail.ttlMillis)} />
				<Fact label="Size" value={`${detail.length.toLocaleString()} ${unit}`} />
				<Fact label="Memory" value={detail.memoryBytes > 0 ? formatBytes(detail.memoryBytes) : "—"} />
			</dl>

			{/* No TTL on a key that looks like a lock is the usual reason to be here:
			    nothing will ever release it. Said once, plainly, rather than guessed
			    at from the name. */}
			{detail.ttlMillis < 0 ? (
				<p className="text-xs text-muted-foreground">
					This key never expires. If it is a lock, nothing will release it but a delete.
				</p>
			) : null}

			{detail.truncated ? (
				<Banner
					variant="info"
					message="Showing the first part only. The panel reads at most 100 items, fewer when they are large, and no more than 4 KiB of any one value (64 KiB of a string), so a large key cannot stall the server or the panel."
				/>
			) : null}

			{detail.supported ? (
				<KeyValue detail={detail} />
			) : (
				<p className="text-sm text-muted-foreground">
					{detail.type} is a module type the panel cannot read. It exists and has the size above.
				</p>
			)}
		</div>
	);
}

function Fact({ label, value }: { label: string; value: string }) {
	return (
		<div>
			<dt className="text-muted-foreground">{label}</dt>
			<dd className="mt-1 font-mono">{value}</dd>
		</div>
	);
}
