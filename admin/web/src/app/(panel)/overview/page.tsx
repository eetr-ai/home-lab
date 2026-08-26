import { CircleCheck, CircleX, LayoutDashboard } from "lucide-react";
import { describeCaller } from "@/app/actions/identity";
import { PageHeader } from "@/components/ui/page-header";
import { Card } from "@/components/ui/card";

/**
 * The panel's landing page, and its end-to-end proof.
 *
 * It reports what the API says about the caller. That single round trip exercises
 * everything the rest of the panel depends on — the operator's token was stored
 * in the session, sent as a bearer credential, verified against eetr-auth's
 * published keys, and its subject read back — so when a section is broken, this
 * page says whether the cause is above or below the API.
 */
export default async function OverviewPage() {
	const caller = await describeCaller();

	return (
		<main className="flex min-h-screen flex-col p-6">
			<PageHeader
				icon={LayoutDashboard}
				title="Overview"
				description="The panel's connection to the admin API."
			/>

			<Card className="max-w-xl">
				<h2 className="mb-4 flex items-center gap-2 text-lg font-medium">
					{caller.ok ? (
						<CircleCheck className="h-5 w-5 text-success-icon" />
					) : (
						<CircleX className="h-5 w-5 text-danger-icon" />
					)}
					Admin API
				</h2>

				{caller.ok ? (
					<dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
						<dt className="text-muted-foreground">Subject</dt>
						<dd className="truncate font-mono">{caller.data.subject}</dd>
						<dt className="text-muted-foreground">Email</dt>
						<dd className="truncate">{caller.data.email || "—"}</dd>
					</dl>
				) : (
					<p className="text-sm text-danger-fg">{caller.error}</p>
				)}
			</Card>
		</main>
	);
}
