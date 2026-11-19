"use client";

import { X } from "lucide-react";
import { CopyButton, IconButton } from "@/components/ui";

/**
 * The statements the last change committed, shown back as SQL.
 *
 * In a <pre> rather than only behind the copy button: CopyButton renders nothing
 * on a plain-HTTP origin, and selecting the text is what still works there.
 */
export function AppliedStatements({ statements, onDismiss }: { statements: string[]; onDismiss: () => void }) {
	if (statements.length === 0) return null;
	const sql = statements.map((one) => `${one};`).join("\n");

	return (
		<div className="rounded-card bg-success-bg px-3 py-2 text-sm text-success-fg">
			<div className="flex items-center justify-between gap-2">
				<span>
					Applied {statements.length === 1 ? "one statement" : `${statements.length} statements`}, in one
					transaction:
				</span>
				<div className="flex items-center gap-1">
					<CopyButton text={sql} label="Copy the statements" />
					<IconButton aria-label="Dismiss" onClick={onDismiss}>
						<X className="h-4 w-4" />
					</IconButton>
				</div>
			</div>
			<pre className="mt-2 overflow-x-auto whitespace-pre-wrap break-all font-mono text-xs">{sql}</pre>
		</div>
	);
}
