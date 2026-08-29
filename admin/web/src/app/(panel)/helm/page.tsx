import { redirect } from "next/navigation";

/** The section has no page of its own; releases are what it is for. */
export default function HelmPage() {
	redirect("/helm/releases");
}
