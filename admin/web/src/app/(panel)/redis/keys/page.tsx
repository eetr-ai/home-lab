import { scanKeys } from "@/app/actions/redis";
import { searchKey } from "@/lib/redis/search";
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
 * Keyed by the search (lib/redis/search.ts): a new search, even of the same
 * pattern, starts the trail over rather than paging with an old cursor, while a
 * revalidation re-renders the same search and keeps the operator's place.
 */
export default async function RedisKeysPage({
	searchParams,
}: {
	searchParams: Promise<{ pattern?: string; t?: string }>;
}) {
	const { pattern = "", t } = await searchParams;
	const first = await scanKeys(pattern, "");
	return <KeyBrowser key={searchKey(pattern, t)} pattern={pattern} first={first} />;
}
