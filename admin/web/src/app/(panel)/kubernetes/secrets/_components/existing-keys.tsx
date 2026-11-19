"use client";

import { RefreshCw, Trash2, Undo2 } from "lucide-react";
import { IconButton, cn } from "@/components/ui";
import type { KeyAction } from "./secret-edit";

/**
 * The keys a Secret already has, each kept, replaced, or removed.
 *
 * Fixed rather than typed: what a key is called is read from the listing, so a
 * key cannot be replaced or removed that the Secret does not have. Replacing one
 * opens a value field below; removing one strikes it through and offers the undo
 * in the same place, so a key marked by mistake is one click from being kept.
 */
export function ExistingKeys({
	keys,
	actionFor,
	onChange,
}: {
	keys: string[];
	actionFor: (key: string) => KeyAction;
	onChange: (key: string, action: KeyAction) => void;
}) {
	return (
		<fieldset>
			<legend className="mb-2 text-sm font-medium">Keys</legend>
			<ul className="divide-y divide-border">
				{keys.map((key) => {
					const action = actionFor(key);
					return (
						<li key={key} className="flex items-center gap-2 py-1.5">
							<code
								className={cn(
									"min-w-0 flex-1 truncate font-mono text-sm",
									action === "remove" && "text-muted-foreground line-through",
								)}
							>
								{key}
							</code>
							<span className="text-xs text-muted-foreground">{LABELS[action]}</span>
							{action === "remove" ? (
								<IconButton
									type="button"
									aria-label={`Keep ${key}`}
									onClick={() => onChange(key, "keep")}
								>
									<Undo2 className="h-4 w-4" />
								</IconButton>
							) : (
								<>
									<IconButton
										type="button"
										aria-label={`Replace the value of ${key}`}
										aria-pressed={action === "replace"}
										onClick={() => onChange(key, action === "replace" ? "keep" : "replace")}
										className={cn(action === "replace" && "bg-surface-sunken text-foreground")}
									>
										<RefreshCw className="h-4 w-4" />
									</IconButton>
									<IconButton
										type="button"
										variant="danger"
										aria-label={`Remove ${key}`}
										onClick={() => onChange(key, "remove")}
									>
										<Trash2 className="h-4 w-4" />
									</IconButton>
								</>
							)}
						</li>
					);
				})}
			</ul>
		</fieldset>
	);
}

const LABELS: Record<KeyAction, string> = {
	keep: "",
	replace: "new value below",
	remove: "will be removed",
};
