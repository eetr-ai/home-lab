"use client";

import { Check } from "lucide-react";
import { Table, TBody, THead, Td, Th } from "@/components/ui";
import type { PostgresObjectPrivileges } from "@/lib/api/types";
import { foreignGrantor } from "@/lib/postgres/privileges";

/**
 * One kind of object as a grid: a row per object, a column per privilege.
 *
 * Presentational. The box shows what is ticked in the draft; whether that
 * differs from what is granted is the parent's business. Two things are drawn
 * beside a box, because both explain a box that does not mean what it seems to:
 *
 * - a muted tick, when the role holds the privilege without it being granted by
 *   name — through PUBLIC or a role it belongs to. Unticking cannot remove that.
 * - "owner", on an object the role owns. An owner is granted everything by
 *   default, so its boxes are disabled rather than offered as something to revoke.
 *
 * A box whose privilege another role granted is locked, with the grantor in its
 * tooltip: the panel's revoke acts as the owner and cannot remove that grant.
 */
export function PrivilegeMatrix({
	role,
	title,
	columns,
	objects,
	qualified,
	ticked,
	editable,
	onToggle,
}: {
	/** The role being shown, so an object it owns can say so. */
	role: string;
	title: string;
	columns: readonly string[];
	objects: PostgresObjectPrivileges[];
	/** Show the schema before the name, for tables and sequences. */
	qualified: boolean;
	ticked: (object: PostgresObjectPrivileges) => string[];
	editable: (object: PostgresObjectPrivileges) => boolean;
	onToggle: (object: PostgresObjectPrivileges, privilege: string) => void;
}) {
	if (objects.length === 0) {
		return (
			<section>
				<h3 className="mb-2 text-sm font-medium">{title}</h3>
				<p className="text-sm text-muted-foreground">None in this database.</p>
			</section>
		);
	}

	return (
		<section>
			<h3 className="mb-2 text-sm font-medium">{title}</h3>
			<Table minWidth="min-w-[640px]">
				<THead>
					<Th>Name</Th>
					{columns.map((privilege) => (
						<Th key={privilege} className="text-center">
							{privilege}
						</Th>
					))}
				</THead>
				<TBody>
					{objects.map((object) => {
						const label = qualified ? `${object.schema}.${object.name}` : object.name;
						const boxes = ticked(object);
						const canEdit = editable(object);
						return (
							<tr key={JSON.stringify([object.schema, object.name])}>
								<Td className="font-medium">
									{label}
									{object.kind !== "table" && object.kind !== "sequence" && qualified ? (
										<span className="ml-2 text-xs text-muted-foreground">{object.kind}</span>
									) : null}
									{object.owner === role ? (
										<span className="ml-2 text-xs text-muted-foreground">owner</span>
									) : null}
								</Td>
								{columns.map((privilege) => {
									const inherited = object.effective.includes(privilege) && !object.granted.includes(privilege);
									const grantor = foreignGrantor(object, privilege);
									return (
										<Td key={privilege} className="text-center">
											<span className="inline-flex items-center gap-1">
												<input
													type="checkbox"
													aria-label={`${privilege} on ${label}`}
													checked={boxes.includes(privilege)}
													disabled={!canEdit || grantor !== undefined}
													title={grantor ? `Granted by ${grantor}; revoke it as ${grantor}` : undefined}
													onChange={() => onToggle(object, privilege)}
													className="h-4 w-4 cursor-pointer rounded-chip border-border accent-brand disabled:cursor-default disabled:opacity-50"
												/>
												{inherited ? (
													<Check
														className="h-3.5 w-3.5 text-muted-foreground"
														aria-label="also held through PUBLIC or a role membership"
													/>
												) : null}
											</span>
										</Td>
									);
								})}
							</tr>
						);
					})}
				</TBody>
			</Table>
		</section>
	);
}
