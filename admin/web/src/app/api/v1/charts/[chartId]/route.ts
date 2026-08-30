import { apiKeyFrom, exchangeForToken } from "@/lib/auth/api-key";
import { adminApiBaseUrl } from "@/lib/api/config";
import { MAX_BODY_BYTES, readBounded } from "@/lib/pipeline/bounded-body";
import { parsePatch } from "@/lib/pipeline/patch-request";

/**
 * The one endpoint a pipeline calls: roll a declared deployment onto a chart
 * version.
 *
 *     CI ──eak_…──▶ this route ──exchange──▶ eetr-auth
 *                        └──JWT──▶ admin API PUT /api/helm/deployments/{id}
 *
 * A route handler rather than a server action, for the reason layer-conventions
 * names: something outside this app has to call it by URL, and a server action is
 * not a URL anybody can write down.
 *
 * Two things about it are deliberate and easy to undo by accident.
 *
 * It does not go through `src/lib/api/http.ts`. `call()` answers every failure as
 * `{ok:false,error}`, and the status is the part a pipeline branches on — a `409`
 * means somebody else is mid-deploy and retrying later is right, where a `400`
 * means retrying is pointless. Collapsing the two would make a CI script that
 * distinguishes them impossible to write.
 *
 * It also never touches `bearerToken()`. That is the signed-in operator's
 * credential; there is no operator here, and reaching for a session on this path
 * would be reaching for somebody else's.
 *
 * Note that `src/proxy.ts` must exempt `/api/v1/` from the session gate, or this
 * handler is never reached at all.
 */

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/** How long the admin API has. Matches the RestClient every other call uses. */
const TIMEOUT_MS = 20_000;

export async function PATCH(
	req: Request,
	{ params }: { params: Promise<{ chartId: string }> },
): Promise<Response> {
	const apiKey = apiKeyFrom(req.headers.get("authorization"));
	if (!apiKey) {
		return refusal("an API key is required: Authorization: Bearer eak_…", 401);
	}

	// Read before the exchange. A malformed body is the caller's mistake either
	// way, and finding it out without a round trip to the identity provider is
	// both faster and one fewer thing in eetr-auth's activity log.
	//
	// Bounded, because that ordering means an unverified caller is the one whose
	// body is being buffered. `req.json()` would read whatever arrives.
	const raw = await readBounded(req);
	if ("tooLarge" in raw) {
		return refusal(`the body is larger than ${MAX_BODY_BYTES} bytes`, 413);
	}

	let body: unknown;
	try {
		body = JSON.parse(raw.text);
	} catch {
		return refusal("the body could not be parsed as JSON", 400);
	}

	const parsed = parsePatch(body);
	if ("error" in parsed) return refusal(parsed.error, 400);

	const base = adminApiBaseUrl();
	if (!base) {
		return refusal("the admin API is not configured (ADMIN_API_URL is unset)", 503);
	}

	const credential = await exchangeForToken(apiKey);
	if ("error" in credential) return refusal(credential.error, credential.status);

	const { chartId } = await params;
	const upstream = `${base}/api/helm/deployments/${encodeURIComponent(chartId)}`;

	try {
		const response = await fetch(upstream, {
			method: "PUT",
			headers: {
				Authorization: `Bearer ${credential.token}`,
				"Content-Type": "application/json",
			},
			body: JSON.stringify(parsed.request),
			signal: AbortSignal.timeout(TIMEOUT_MS),
			cache: "no-store",
		});

		// The API's own body and status, relayed rather than reinterpreted. It says
		// which job was created, or why it refused, in prose this route could only
		// make worse — and a pipeline reading `.job` out of the 202 is reading the
		// admin API's answer, not this endpoint's paraphrase of it.
		return new Response(await response.text(), {
			status: response.status,
			headers: { "Content-Type": "application/json; charset=utf-8" },
		});
	} catch (err) {
		return refusal(`the admin API is unreachable: ${(err as Error).message}`, 502);
	}
}

/**
 * A refusal shaped like the admin API's own `{error, message}`.
 *
 * One shape for every failure on this path, whether it came from here or from
 * upstream, so a pipeline has one thing to parse.
 */
function refusal(message: string, status: number): Response {
	return Response.json({ error: "request_refused", message }, { status });
}
