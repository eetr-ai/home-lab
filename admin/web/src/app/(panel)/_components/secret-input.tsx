"use client";

import { useId, useRef, useState, useTransition } from "react";
import { Eye, EyeOff, WandSparkles } from "lucide-react";
import { Button, IconButton, Input, Popover, Select, cn, CopyButton } from "@/components/ui";
import type { InputProps } from "@/components/ui";
import { generateSecretValue } from "@/app/actions/tools";
import {
	DEFAULT_LENGTH,
	MAX_LENGTH,
	MIN_LENGTH,
	PRESETS,
	isValidLength,
	type Preset,
} from "@/lib/secrets/generate";

/**
 * A field for a value nobody should be inventing.
 *
 * Here rather than in components/ui, and that is the layering rather than an
 * accident: it calls a server action, so it knows about this application's API.
 * The primitives it is built from — Input, Popover, CopyButton — do not, and
 * keeping them that way is what makes them reusable.
 *
 * Two controls beside the input: reveal, because a password you cannot see is a
 * password you cannot check you pasted correctly, and generate, which opens a
 * popover offering the four shapes worth having.
 *
 * The generated value is shown in the popover before it is used, with a copy
 * button, because of the order these things happen in: you generate a database
 * password, install it as a Secret, and then need the same string for a values
 * file or a colleague. There is no reading it back afterwards — the API never
 * returns a Secret's value — so the moment it is on screen is the only moment to
 * take it.
 */
export interface SecretInputProps extends Omit<InputProps, "type" | "value" | "onChange"> {
	value: string;
	onChange: (value: string) => void;
	/** What a generated value is for, shown as the popover's heading. */
	generateLabel?: string;
}

export function SecretInput({
	value,
	onChange,
	generateLabel = "Generate a value",
	className,
	id,
	...rest
}: SecretInputProps) {
	const [revealed, setRevealed] = useState(false);
	const [generating, setGenerating] = useState(false);
	const trigger = useRef<HTMLButtonElement>(null);

	return (
		<div className="flex items-center gap-1">
			<Input
				id={id}
				type={revealed ? "text" : "password"}
				value={value}
				onChange={(event) => onChange(event.target.value)}
				// A generated value is not one the browser should offer to save over,
				// and a password manager filling this in would overwrite a fresh one.
				autoComplete="new-password"
				spellCheck={false}
				className={cn("font-mono", className)}
				{...rest}
			/>

			<IconButton
				type="button"
				aria-label={revealed ? "Hide the value" : "Show the value"}
				aria-pressed={revealed}
				onClick={() => setRevealed(!revealed)}
			>
				{revealed ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
			</IconButton>

			<IconButton
				ref={trigger}
				type="button"
				aria-label={generateLabel}
				aria-expanded={generating}
				onClick={() => setGenerating(!generating)}
			>
				<WandSparkles className="h-4 w-4" />
			</IconButton>

			<GeneratorPopover
				open={generating}
				anchor={trigger}
				title={generateLabel}
				onRequestClose={() => setGenerating(false)}
				onUse={(generated) => {
					onChange(generated);
					// Revealed on use, deliberately. A field that fills with dots gives
					// no sign it took the value you just looked at.
					setRevealed(true);
					setGenerating(false);
				}}
			/>
		</div>
	);
}

/**
 * The generator itself.
 *
 * A `Popover` rather than a dialog because it is not a decision to stop for: the
 * form behind it is still the subject, and dismissing this leaves the field
 * exactly as it was. Nothing here touches the field until "Use this value".
 */
function GeneratorPopover({
	open,
	anchor,
	title,
	onRequestClose,
	onUse,
}: {
	open: boolean;
	anchor: React.RefObject<HTMLButtonElement | null>;
	title: string;
	onRequestClose: () => void;
	onUse: (value: string) => void;
}) {
	const [preset, setPreset] = useState<Preset>("password");
	const [length, setLength] = useState(DEFAULT_LENGTH);
	// The candidate, held rather than derived. A value that regenerated on every
	// render could not be copied — the string in the clipboard would stop being
	// the string on screen.
	const [candidate, setCandidate] = useState<string | null>(null);
	const [error, setError] = useState<string | null>(null);
	const [pending, startTransition] = useTransition();
	const presetId = useId();
	const lengthId = useId();

	const chosen = PRESETS.find((option) => option.id === preset) ?? PRESETS[0];

	function mint(nextPreset: Preset = preset, nextLength: number = length) {
		// The bounds are checked here as well as in the API so a mistyped length is
		// a message under the field rather than a round trip that comes back 400.
		if (!isValidLength(nextLength)) {
			setCandidate(null);
			setError(`Length must be a whole number between ${MIN_LENGTH} and ${MAX_LENGTH}.`);
			return;
		}

		startTransition(async () => {
			const result = await generateSecretValue(nextPreset, nextLength);
			if (result.ok) {
				setCandidate(result.data.value);
				setError(null);
				return;
			}
			// The previous candidate is cleared, so a failed regeneration cannot
			// leave a value on screen that the message beside it contradicts.
			setCandidate(null);
			setError(result.error);
		});
	}

	return (
		<Popover open={open} onRequestClose={onRequestClose} anchor={anchor} title={title} width="sm">
			<div className="space-y-3 p-4">
				<div className="space-y-1">
					<label htmlFor={presetId} className="text-xs font-medium text-muted-foreground">
						Shape
					</label>
					<Select
						id={presetId}
						className="w-full"
						value={preset}
						onChange={(event) => {
							const next = event.target.value as Preset;
							setPreset(next);
							// Regenerate rather than clear: the candidate on screen is the
							// answer to a question nobody is asking any more.
							if (candidate || error) mint(next);
						}}
					>
						{PRESETS.map((option) => (
							<option key={option.id} value={option.id}>
								{option.label}
							</option>
						))}
					</Select>
					<p className="text-xs text-muted-foreground">{chosen.hint}</p>
				</div>

				{chosen.sized ? (
					<div className="space-y-1">
						<label htmlFor={lengthId} className="text-xs font-medium text-muted-foreground">
							Length
						</label>
						<Input
							id={lengthId}
							type="number"
							min={MIN_LENGTH}
							max={MAX_LENGTH}
							value={length}
							onChange={(event) => {
								const next = Number(event.target.value);
								setLength(next);
								if (candidate || error) mint(preset, next);
							}}
						/>
					</div>
				) : null}

				{error ? <p className="text-xs text-danger">{error}</p> : null}

				{candidate ? (
					<div className="flex items-start gap-1">
						{/* Selectable, always. The copy button is absent over plain HTTP,
						    and selecting this is what still works there. */}
						<pre className="min-w-0 flex-1 overflow-x-auto rounded-control bg-surface-sunken p-2 font-mono text-xs">
							<code className="break-all whitespace-pre-wrap">{candidate}</code>
						</pre>
						<CopyButton text={candidate} label="Copy the generated value" />
					</div>
				) : null}

				<div className="flex justify-end gap-2">
					<Button type="button" variant="secondary" loading={pending} onClick={() => mint()}>
						{candidate ? "Again" : "Generate"}
					</Button>
					<Button
						type="button"
						disabled={!candidate || pending}
						onClick={() => candidate && onUse(candidate)}
					>
						Use this value
					</Button>
				</div>
			</div>
		</Popover>
	);
}
