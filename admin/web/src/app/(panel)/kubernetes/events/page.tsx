import { CalendarClock } from "lucide-react";
import { listEvents } from "@/app/actions/kube";
import { Td, Th } from "@/components/ui/table";
import { Directory } from "../../_components/directory";
import { ScopePicker } from "../../_components/scope-picker";
import { resolveNamespace } from "../_components/namespace-scope";
import { formatAge } from "@/lib/format/age";

export const dynamic = "force-dynamic";

export default async function EventsPage({
	searchParams,
}: {
	searchParams: Promise<{ namespace?: string }>;
}) {
	const { namespace: requested } = await searchParams;
	const { namespaces, selected, error } = await resolveNamespace(requested);
	const events = selected ? await listEvents(selected) : null;
	const rows = events?.ok ? events.data : [];
	const now = new Date();

	return (
		<>
			<ScopePicker label="Namespace" param="namespace" options={namespaces} selected={selected} />
			<Directory
				error={error ?? (events && !events.ok ? events.error : null)}
				isEmpty={rows.length === 0}
				minWidth="min-w-[860px]"
				empty={{
					icon: CalendarClock,
					title: "No recent events",
					// Worth saying, because an empty list here reads as "nothing is wrong"
					// when it can equally mean "whatever went wrong has aged out".
					description: "The cluster keeps events for about an hour, so a quiet namespace looks the same as an old problem.",
				}}
				columns={
					<>
						<Th>Type</Th>
						<Th>Reason</Th>
						<Th>Object</Th>
						<Th>Message</Th>
						<Th className="text-right">Count</Th>
						<Th className="text-right">Last seen</Th>
					</>
				}
				rows={rows.map((event, index) => (
					// The API does not surface the event's own name, and one object can
					// produce several events with the same reason, so the index is part of
					// the key rather than a lazy substitute for one.
					<tr key={`${event.object}-${event.reason}-${index}`}>
						<Td className={event.type === "Warning" ? "text-warning-fg" : "text-muted-foreground"}>
							{event.type}
						</Td>
						<Td className="font-medium">{event.reason}</Td>
						<Td className="text-muted-foreground">{event.object}</Td>
						<Td className="max-w-md truncate text-muted-foreground" title={event.message}>
							{event.message}
						</Td>
						<Td className="text-right text-muted-foreground">{event.count}</Td>
						<Td className="text-right text-muted-foreground">{formatAge(event.lastSeen, now)}</Td>
					</tr>
				))}
			/>
		</>
	);
}
