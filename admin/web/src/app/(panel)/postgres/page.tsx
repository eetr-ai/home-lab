import { redirect } from "next/navigation";

/** The section has no landing page of its own; its first tab is the landing page. */
export default function PostgresPage() {
	redirect("/postgres/databases");
}
