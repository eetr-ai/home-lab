import Link from "next/link";

/**
 * The way out of a detail page.
 *
 * Every page reached by clicking a row needs one, and it is easy to leave off
 * because the browser's own back button exists — but that only helps somebody who
 * arrived by clicking. A detail page reached from a bookmark, a link in chat, or
 * the assistant's `navigate_to` is otherwise a dead end with no way to see what
 * else there is.
 *
 * A link and not a button: it navigates, so middle-click and open-in-new-tab
 * should work.
 */
export function BackLink({ href, label }: { href: string; label: string }) {
	return (
		<Link href={href} className="text-sm text-muted-foreground hover:text-foreground">
			← {label}
		</Link>
	);
}
