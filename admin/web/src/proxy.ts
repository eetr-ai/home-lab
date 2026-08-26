import { NextResponse } from "next/server";
import type { ProxyConfig } from "next/server";
import { auth } from "@/auth";

/**
 * Next.js proxy (what earlier versions called middleware): the session gate every
 * navigation passes through.
 *
 * It does two jobs. The obvious one is refusing unauthenticated requests. The
 * quieter one is that running `auth()` here is what lets a refreshed token pair
 * actually be stored: a Server Component cannot write cookies, so a renewal that
 * happened during a page render would be thrown away and recomputed on the next
 * request. The proxy and the server actions can write, and between them they
 * cover every path that reaches the API.
 *
 * A session whose refresh failed counts as no session at all, so a revoked
 * refresh token sends the operator to sign in rather than to a page that can load
 * nothing.
 */

/** Paths reachable without a session: the sign-in page, Auth.js, and assets. */
function isPublic(pathname: string): boolean {
	return pathname === "/" || pathname.startsWith("/api/auth");
}

export default auth((req) => {
	const { pathname, search } = req.nextUrl;
	if (isPublic(pathname)) return NextResponse.next();
	if (req.auth && req.auth.error === undefined) return NextResponse.next();

	// A redirect is the right refusal for a navigation and the wrong one for a
	// fetch: the browser follows it, and the caller gets the sign-in page's HTML
	// as a 200 it will try to parse. The log viewer would render markup as log
	// lines, and `res.ok` would be true the whole time. API routes get a status
	// they can branch on instead.
	if (pathname.startsWith("/api/")) {
		return NextResponse.json({ error: "not signed in" }, { status: 401 });
	}

	// Carry the original path so sign-in returns there rather than to the root.
	const url = new URL("/", req.nextUrl.origin);
	url.searchParams.set("callbackUrl", `${pathname}${search}`);
	return NextResponse.redirect(url);
});

export const config: ProxyConfig = {
	// Everything except Next internals and static files (which have a dot).
	matcher: ["/((?!_next/static|_next/image|.*\\..*).*)"],
};
