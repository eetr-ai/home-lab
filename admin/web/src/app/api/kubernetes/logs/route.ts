import { denyUnlessSignedIn } from "@/app/actions/_auth";
import { adminApiBaseUrl, bearerToken } from "@/lib/api/config";

/**
 * The pod log stream, proxied to the admin API.
 *
 * A route handler rather than a server action, and not for lack of trying: a
 * server action's return value is serialized as one RSC payload, so there is no
 * way to deliver a line before the last one arrives. A log tail has no last one.
 *
 * It is also why this cannot go through `src/lib/api/http.ts` — `call()` buffers
 * the whole response and the RestClient gives up after twenty seconds. The base
 * URL and the operator's token come from `lib/api/config.ts`, which both share,
 * so there is one definition of how this app reaches the API.
 *
 * `signal: req.signal` is the load-bearing line. The browser aborting — a closed
 * panel, a navigation — cancels this fetch, which drops the connection to the
 * admin API, which cancels its request context, which tears the stream down at
 * the API server. Without it a closed tab leaks a goroutine and a connection per
 * pod anyone ever looked at.
 */

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/** Nginx-style: the client hung up before a response was finished. */
const CLIENT_CLOSED_REQUEST = 499;

/** What a caller may pass through, so nothing else reaches the API untouched. */
const FORWARDED = ["container", "follow", "tail", "previous"] as const;

export async function GET(req: Request): Promise<Response> {
	const denied = await denyUnlessSignedIn();
	if (denied) return Response.json({ error: denied.error }, { status: 401 });

	const url = new URL(req.url);
	const namespace = url.searchParams.get("namespace");
	const pod = url.searchParams.get("pod");
	if (!namespace || !pod) {
		return Response.json({ error: "namespace and pod are required" }, { status: 400 });
	}

	const base = adminApiBaseUrl();
	if (!base) {
		return Response.json(
			{ error: "the admin API is not configured (ADMIN_API_URL is unset)" },
			{ status: 503 },
		);
	}

	const credential = await bearerToken();
	if ("error" in credential) {
		return Response.json({ error: credential.error }, { status: 401 });
	}

	const upstream = new URL(
		`${base}/api/kubernetes/namespaces/${encodeURIComponent(namespace)}` +
			`/pods/${encodeURIComponent(pod)}/logs`,
	);
	for (const param of FORWARDED) {
		const value = url.searchParams.get(param);
		if (value !== null) upstream.searchParams.set(param, value);
	}

	try {
		const response = await fetch(upstream, {
			headers: { Authorization: `Bearer ${credential.token}` },
			signal: req.signal,
			cache: "no-store",
		});

		if (!response.ok) {
			// The API's own prose, not a bare status: it says which container the
			// pod wanted, or that the ServiceAccount may not read logs at all.
			const detail = await response.text();
			return new Response(detail || `the admin API answered ${response.status}`, {
				status: response.status,
				headers: { "Content-Type": "text/plain; charset=utf-8" },
			});
		}

		// A success with no body is a pod that has logged nothing, which is a real
		// answer. Folding it into the branch above would render "the admin API
		// answered 200" as the first line of the log.
		if (!response.body) {
			return new Response("", {
				status: 200,
				headers: { "Content-Type": "text/plain; charset=utf-8" },
			});
		}

		return new Response(response.body, {
			status: 200,
			headers: {
				"Content-Type": "text/plain; charset=utf-8",
				"Cache-Control": "no-cache, no-transform",
				// Belt and braces with the API's own header: nothing between here and
				// the browser may buffer the tail into batches.
				"X-Accel-Buffering": "no",
			},
		});
	} catch (err) {
		// An abort is how a follow stream normally ends — the operator closed the
		// panel — so it is not an error to report anywhere.
		if (err instanceof Error && err.name === "AbortError") {
			return new Response(null, { status: CLIENT_CLOSED_REQUEST });
		}
		return new Response(`the admin API is unreachable: ${(err as Error).message}`, {
			status: 502,
			headers: { "Content-Type": "text/plain; charset=utf-8" },
		});
	}
}
