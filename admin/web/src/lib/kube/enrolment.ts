import type { Namespace } from "@/lib/api/types";

/**
 * What a namespace's Helm enrolment means in the panel: a word, a tone, and
 * which action to offer.
 *
 * This is a projection rather than a component, so it can be tested without
 * React — and the rule worth testing is the one about `wrong`. A namespace whose
 * bindings point somewhere an older chart left them keeps failing deploys and
 * looks fine: nothing about "partly set up" or a green tick would say so, which
 * is why it is its own state with its own word.
 */
export type Enrolment = "enrolled" | "partial" | "wrong" | "missing" | "unknown";

export interface EnrolmentView {
	/** The word in the column. */
	label: string;
	tone: "ok" | "warn" | "muted";
	/** The button, or null when there is nothing to offer. */
	action: { label: string; danger: boolean } | null;
}

const VIEWS: Record<Enrolment, EnrolmentView> = {
	enrolled: {
		label: "set up",
		tone: "ok",
		// Revoking is what makes enrolment reversible, which is what makes it safe
		// to offer at all. Marked as a danger because it takes the panel's access
		// to a namespace away, and a running release becomes unreadable.
		action: { label: "Revoke", danger: true },
	},
	partial: { label: "partly set up", tone: "warn", action: { label: "Repair", danger: false } },
	wrong: { label: "set up wrongly", tone: "warn", action: { label: "Repair", danger: false } },
	missing: { label: "not set up", tone: "muted", action: { label: "Set up", danger: false } },
	// Nothing is offered on unknown. The bindings could not be read, so pressing
	// a button would be acting on an answer the panel does not have.
	unknown: { label: "unknown", tone: "muted", action: null },
};

/**
 * The view for a namespace, or null when there is nothing to show.
 *
 * Null for two different reasons, and both are right. A protected namespace is
 * never a Helm target, so offering to set one up would be offering something the
 * API refuses. And a namespace with no enrolment state has not asked to be one —
 * the API leaves the field off — which is not the same as being set up wrongly.
 */
export function enrolmentView(namespace: Namespace): EnrolmentView | null {
	if (namespace.protected) return null;
	if (!namespace.helmEnrolment) return null;
	return VIEWS[namespace.helmEnrolment as Enrolment] ?? null;
}
