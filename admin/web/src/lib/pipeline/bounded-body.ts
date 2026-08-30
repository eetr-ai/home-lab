/**
 * Reading a request body without letting the sender choose how much memory it
 * costs.
 *
 * This matters here and not on the panel's other routes: `/api/v1/` is reached
 * without a session, and the API key on it is not verified until after the body
 * has been read — checking it first would mean a round trip to eetr-auth for
 * every malformed request anybody sends. So anything that can spell `eak_` can
 * start a body, and `req.json()` would buffer whatever arrives.
 *
 * `Content-Length` is checked because it is cheap and refuses the honest case
 * early, and the stream is counted anyway because a chunked request does not have
 * to declare one, and a dishonest one may lie.
 */

/**
 * The most a pipeline may send.
 *
 * Generous against the real limit, which is the API's own 256 KiB cap on a values
 * document — this is not a second opinion about what is reasonable to deploy, it
 * is a bound on what one request can cost before anybody has been authenticated.
 * A body between the two is refused by the API, which is the layer that should
 * say so.
 */
export const MAX_BODY_BYTES = 512 * 1024;

/** The body as text, or the fact that it was too big to read. */
export type BoundedBody = { text: string } | { tooLarge: true };

export async function readBounded(
	request: Request,
	limit: number = MAX_BODY_BYTES,
): Promise<BoundedBody> {
	const declared = Number(request.headers.get("content-length"));
	if (Number.isFinite(declared) && declared > limit) return { tooLarge: true };

	const reader = request.body?.getReader();
	if (!reader) return { text: "" };

	const chunks: Uint8Array[] = [];
	let size = 0;

	for (;;) {
		const { done, value } = await reader.read();
		if (done) break;
		size += value.byteLength;
		if (size > limit) {
			// Hang up rather than draining politely: the point is to stop paying for
			// this request, and reading the rest to be tidy is paying for it.
			await reader.cancel();
			return { tooLarge: true };
		}
		chunks.push(value);
	}

	const joined = new Uint8Array(size);
	let at = 0;
	for (const chunk of chunks) {
		joined.set(chunk, at);
		at += chunk.byteLength;
	}
	return { text: new TextDecoder().decode(joined) };
}
