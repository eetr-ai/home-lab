import { redirect } from "next/navigation";
import { ShieldCheck } from "lucide-react";
import { auth } from "@/auth";
import { startSignIn } from "@/app/actions/session";
import { PROVIDER_NAME } from "@/lib/auth/oidc-config";
import { signInErrorMessage } from "@/lib/auth/sign-in-error";
import { Banner } from "@/components/ui/banner";
import { Card } from "@/components/ui/card";
import { SignInButton } from "./sign-in-button";

/**
 * The sign-in page, and the only route reachable without a session.
 *
 * A Server Component with a plain `<form>`: the whole flow is a redirect to
 * eetr-auth and back, so there is no client session to hydrate. The one client
 * piece is the submit button, which shows a spinner while the redirect is on its
 * way. The UI primitives are imported by module rather than
 * through the barrel, which pulls in the hook-using overlays — see
 * src/components/ui/index.ts.
 */
export default async function SignInPage({
	searchParams,
}: {
	searchParams: Promise<{ callbackUrl?: string; error?: string }>;
}) {
	const { callbackUrl, error } = await searchParams;
	const session = await auth();
	// A session whose refresh failed is not a session; leaving it here would bounce
	// the operator straight back with nothing explained.
	if (session?.user && session.error === undefined) redirect(callbackUrl ?? "/overview");

	return (
		<main className="flex min-h-screen items-center justify-center bg-background p-6 text-foreground">
			<Card className="w-full max-w-sm">
				<div className="mb-6 flex flex-col items-center gap-3 text-center">
					<ShieldCheck className="h-8 w-8 text-brand" />
					<div>
						<h1 className="text-lg font-semibold">Home Lab Admin</h1>
						<p className="mt-1 text-sm text-muted-foreground">
							The databases and the cluster, in one place.
						</p>
					</div>
				</div>

				<Banner variant="error" message={signInErrorMessage(error)} />

				<form action={startSignIn}>
					<input type="hidden" name="callbackUrl" value={callbackUrl ?? "/overview"} />
					<SignInButton providerName={PROVIDER_NAME} />
				</form>
			</Card>
		</main>
	);
}
