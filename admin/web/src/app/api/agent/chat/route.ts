import { auth } from "@/auth";
import { denyUnlessSignedIn } from "@/app/actions/_auth";
import { agentBaseUrl } from "@/lib/api/agent";
import { bearerToken } from "@/lib/api/config";
import { permitsWrite, writeAllowlist } from "@/lib/auth/write-access";

/**
 * The conversation, proxied to the agent.
 *
 * A route handler rather than a server action, for the reason already written down
 * in `api/kubernetes/logs/route.ts`: a server action's return value is serialized
 * as one RSC payload, so there is no way to deliver a token before the last one
 * arrives — and an answer that appears all at once, a minute later, is not a chat.
 *
 * `signal: req.signal` is the load-bearing line, and it does more here than it does
 * for a log tail. The browser aborting — a closed drawer, a Stop — cancels this
 * fetch, which drops the connection to the agent, which its `sse-event` block sees
 * (`ifClosed: stop`) and ends the model run. Without it, closing the drawer leaves
 * a model generating for nobody, at full price.
 */

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/** Nginx-style: the client hung up before a response was finished. */
const CLIENT_CLOSED_REQUEST = 499;

export async function POST(req: Request): Promise<Response> {
	const denied = await denyUnlessSignedIn();
	if (denied) return Response.json({ error: denied.error }, { status: 401 });

	const session = await auth();

	// Write access, not merely a session, and the reason is the agent's own
	// capabilities rather than what it may call. Its API tool is GET-only, but it
	// also holds curl inside the cluster and a workspace on a shared volume — so it
	// is gated by the same allowlist every other change in this panel is. An
	// operator given the panel read-only sees no launcher and gets no chat.
	if (!permitsWrite(writeAllowlist(process.env.ADMIN_WRITE_EMAILS), session?.user?.email)) {
		return Response.json(
			{ error: "this account may view the panel but not use the assistant" },
			{ status: 403 },
		);
	}

	const base = agentBaseUrl();
	if (!base) {
		return Response.json({ error: "no assistant is configured here" }, { status: 503 });
	}

	const credential = await bearerToken();
	if ("error" in credential) {
		return Response.json({ error: credential.error }, { status: 401 });
	}

	let parsed: unknown;
	try {
		parsed = await req.json();
	} catch {
		return Response.json({ error: "the request body is not JSON" }, { status: 400 });
	}
	// `req.json()` resolves for any valid JSON *value*, so null, a number and an
	// array all get here — and the cast to a record hid that. A body of `null`
	// then threw on `body.stop` below, which the catch around the fetch reported
	// as 502 "the assistant is unreachable": a bad request from the caller,
	// blamed on the agent.
	if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
		return Response.json({ error: "the request body must be a JSON object" }, { status: 400 });
	}
	const body = parsed as Record<string, unknown>;

	// Written last, so a forged identity in the request body cannot survive. The
	// agent keys its memory and its remembered facts on this, so a `user` a caller
	// chose would be a way to read somebody else's conversation.
	//
	// The id and not the email: the id is what scopes the memory, and the name is
	// the only part the prompt has any use for. Sending the address as well would
	// hand a real person's email to a model provider for nothing.
	const payload = {
		...body,
		user: { id: session?.user?.id ?? "", name: session?.user?.name ?? "" },
	};

	try {
		const upstream = await fetch(`${base}/chat`, {
			method: "POST",
			headers: {
				"Content-Type": "application/json",
				// The credential the agent's admin_read calls the API with. A header
				// rather than a body field so it lands in the flow's `vars` and never in
				// `body` — nothing that reaches the model's input, its memory or a trace.
				"X-Operator-Token": credential.token,
				// `=== true` strictly. A body carrying `stop: "false"` — or any other
				// truthy thing that is not a boolean — must not end somebody's run.
				...(body.stop === true ? { "X-Agent-Stop": "1" } : {}),
			},
			body: JSON.stringify(payload),
			signal: req.signal,
			cache: "no-store",
		});

		if (!upstream.ok) {
			await upstream.body?.cancel();
			return Response.json(
				{ error: `the assistant answered ${upstream.status}` },
				{ status: upstream.status },
			);
		}

		// A run whose message was handed to another run answers 200 with an empty
		// body, and that is a real outcome rather than a failure — the reader loop
		// reads zero frames and says so. Folding it into the branch above would
		// report it as an error the operator cannot act on.
		return new Response(upstream.body, {
			status: 200,
			headers: {
				"Content-Type": "text/event-stream; charset=utf-8",
				"Cache-Control": "no-cache, no-transform",
				Connection: "keep-alive",
				// Belt and braces with the agent's own headers: nothing between here and
				// the browser may buffer a token stream into batches.
				"X-Accel-Buffering": "no",
			},
		});
	} catch (err) {
		// An abort is how a closed drawer and Stop both end a run. It is not an
		// error to report anywhere.
		if (err instanceof Error && err.name === "AbortError") {
			return new Response(null, { status: CLIENT_CLOSED_REQUEST });
		}
		return Response.json(
			{ error: `the assistant is unreachable: ${(err as Error).message}` },
			{ status: 502 },
		);
	}
}
