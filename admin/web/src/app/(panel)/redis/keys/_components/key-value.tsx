import { Table, TBody, Td, Th, THead } from "@/components/ui/table";
import { displayText } from "@/lib/redis/text";
import type { RedisField, RedisKeyDetail } from "@/lib/api/redis-types";

/**
 * A key's value, laid out by its type.
 *
 * One table shape for everything with pairs (hash, sorted set, stream entries)
 * and a list for everything without. Values are shown as Redis holds them —
 * text as text, bytes as marked base64 — and never parsed: a value that looks
 * like JSON is a string to Redis, and pretty-printing it would show something
 * the server does not have.
 */
export function KeyValue({ detail }: { detail: RedisKeyDetail }) {
	if (detail.string) {
		return <pre className={PRE}>{displayText(detail.string)}</pre>;
	}
	if (detail.items) {
		return (
			<ol className="divide-y divide-border rounded-card border border-border">
				{detail.items.map((item, index) => (
					<li key={index} className="flex gap-3 px-4 py-2 font-mono text-sm">
						<span className="w-8 shrink-0 text-right text-muted-foreground">{index}</span>
						<span className="min-w-0 break-all">{displayText(item)}</span>
					</li>
				))}
			</ol>
		);
	}
	if (detail.fields) {
		return <Pairs heading={["Field", "Value"]} rows={detail.fields.map(fieldRow)} />;
	}
	if (detail.members) {
		return (
			<Pairs
				heading={["Member", "Score"]}
				rows={detail.members.map((member) => [displayText(member.member), String(member.score)])}
			/>
		);
	}
	if (detail.entries) {
		return (
			<div className="space-y-3">
				{detail.entries.map((entry) => (
					<div key={entry.id}>
						<p className="mb-1 font-mono text-xs text-muted-foreground">{entry.id}</p>
						<Pairs heading={["Field", "Value"]} rows={entry.fields.map(fieldRow)} />
					</div>
				))}
			</div>
		);
	}
	return <p className="text-sm text-muted-foreground">Empty.</p>;
}

const PRE = "max-h-[60vh] overflow-auto whitespace-pre-wrap break-all rounded-card bg-surface-sunken p-4 font-mono text-sm";

function fieldRow(field: RedisField): [string, string] {
	return [displayText(field.name), displayText(field.value)];
}

function Pairs({ heading, rows }: { heading: [string, string]; rows: [string, string][] }) {
	return (
		<Table>
			<THead>
				<Th>{heading[0]}</Th>
				<Th>{heading[1]}</Th>
			</THead>
			<TBody>
				{rows.map(([left, right], index) => (
					<tr key={index}>
						<Td className="align-top font-mono break-all">{left}</Td>
						<Td className="align-top font-mono break-all">{right}</Td>
					</tr>
				))}
			</TBody>
		</Table>
	);
}
