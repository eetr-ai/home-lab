import { scanKeys } from "@/app/actions/redis";
import { KeyBrowser } from "./_components/key-browser";

export const dynamic = "force-dynamic";

/**
 * The key browser.
 *
 * The pattern lives in the query string, so a filtered view has an address and
 * the back button undoes a search. The first page is read here; paging past it
 * happens in the browser, because SCAN's cursor only moves forward and the trail
 * back is the client's to keep (lib/query/pagination.ts).
 *
 * Keyed by the pattern so a new search starts the trail over rather than paging
 * a new pattern with an old cursor, which SCAN would accept and answer wrongly.
 */
export default async function RedisKeysPage({
	searchParams,
}: {
	searchParams: Promise<{ pattern?: string }>;
}) {
	const { pattern = "" } = await searchParams;
	const first = await scanKeys(pattern, "");
	return <KeyBrowser key={pattern} pattern={pattern} first={first} />;
}
