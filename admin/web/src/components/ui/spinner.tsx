import { Loader2 } from "lucide-react";
import { cn } from "./cn";

export interface SpinnerProps {
	className?: string;
}

/** Spinning loader. Defaults to row size (h-4 w-4). */
export function Spinner({ className }: SpinnerProps) {
	return <Loader2 className={cn("h-4 w-4 animate-spin", className)} />;
}

/** Centered full-page loader for initial data loads. */
export function FullPageSpinner() {
	return (
		<div className="flex min-h-screen items-center justify-center p-6">
			<Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
		</div>
	);
}
