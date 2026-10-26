import { DatabaseBackup } from "lucide-react";
import { PageHeader } from "@/components/ui/page-header";

/**
 * The Backups section: one Google Drive connection, and the schedules that use
 * it. No tabs — a schedule's runs and files are its own page, one level down.
 */
export default function BackupsLayout({ children }: { children: React.ReactNode }) {
	return (
		<main className="flex min-h-screen flex-col p-6">
			<PageHeader
				icon={DatabaseBackup}
				title="Backups"
				description="Scheduled dumps of the lab's PostgreSQL and MongoDB databases, uploaded to a Google Drive folder you choose. Each schedule keeps the newest backups of each database and moves the rest to Drive's trash."
			/>
			{children}
		</main>
	);
}
