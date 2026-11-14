"use client";

import { useEffect } from "react";
import { useFormStatus } from "react-dom";
import { LogIn } from "lucide-react";
import { Button } from "@/components/ui/button";

/**
 * The sign-in submit button, spinning from the click until the browser leaves for
 * eetr-auth.
 *
 * The round trip through the server action and the provider's redirect takes long
 * enough that a still button reads as a dead one, and invites a second click that
 * starts a second authorization flow. `useFormStatus` reads the pending state of
 * the enclosing `<form>`, so the page itself stays a Server Component.
 */
export function SignInButton({ providerName }: { providerName: string }) {
	const { pending } = useFormStatus();

	// Coming back from eetr-auth with the back button restores this page from the
	// back/forward cache exactly as we left it, spinner included, and the form's
	// pending state never settles. We reload so the button is usable again.
	useEffect(() => {
		const onPageShow = (event: PageTransitionEvent) => {
			if (event.persisted) window.location.reload();
		};
		window.addEventListener("pageshow", onPageShow);
		return () => window.removeEventListener("pageshow", onPageShow);
	}, []);

	return (
		<Button type="submit" icon={LogIn} loading={pending} className="w-full justify-center">
			Sign in with {providerName}
		</Button>
	);
}
