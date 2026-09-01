"use client";

import { useEffect, useId, useRef, useState, useTransition } from "react";
import { WandSparkles } from "lucide-react";
import {
	Button,
	CopyAccessory,
	IconButton,
	Input,
	Popover,
	RevealAccessory,
	SecretField,
	Select,
	useSecretField,
} from "@/components/ui";
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
 * A field for a value nobody should be inventing: reveal, and a generator.
 *
 * This is the composition, not the mechanism. `SecretField` in components/ui owns
 * the box and publishes the context; the accessories read it. This module exists
 * because one of those accessories — the generator — calls a server action, and
 * so knows about this application's API in a way a primitive must not.
 *
 * Use it wherever an operator would otherwise type a password in.
 */
export function SecretInput({
	value,
	onChange,
	label = "the password",
	generateLabel = "Generate a value",
	id,
	placeholder,
	required,
}: {
	value: string;
	onChange: (value: string) => void;
	/** Names this field in the accessories' labels, so several on a page differ. */
	label?: string;
	generateLabel?: string;
	id?: string;
	placeholder?: string;
	required?: boolean;
}) {
	return (
		<SecretField
			value={value}
			onChange={onChange}
			label={label}
			id={id}
			placeholder={placeholder}
			required={required}
		>
			<RevealAccessory />
			<GenerateAccessory label={generateLabel} />
		</SecretField>
	);
}

/**
 * Mint a value and put it in the field.
 *
 * An accessory rather than a prop-wired button: it reads the field it is in from
 * context, so it is the same component here and anywhere else a generated value
 * belongs.
 */
export function GenerateAccessory({ label = "Generate a value" }: { label?: string }) {
	const field = useSecretField();
	const [open, setOpen] = useState(false);
	const trigger = useRef<HTMLButtonElement>(null);

	return (
		<>
			<IconButton
				ref={trigger}
				type="button"
				aria-label={label}
				aria-expanded={open}
				onClick={() => setOpen(!open)}
			>
				<WandSparkles className="h-4 w-4" />
			</IconButton>

			<GeneratorPopover
				open={open}
				anchor={trigger}
				title={label}
				onRequestClose={() => setOpen(false)}
				onUse={(generated) => {
					field.setValue(generated);
					// Revealed on use, deliberately. A field that fills with dots gives
					// no sign it took the value you just looked at.
					field.setRevealed(true);
					setOpen(false);
				}}
			/>
		</>
	);
}

/**
 * The generator.
 *
 * A `Popover` rather than a dialog because it is not a decision to stop for: the
 * form behind it is still the subject, and dismissing this leaves the field
 * exactly as it was. Nothing here touches the field until "Use this value".
 *
 * The candidate is shown in a `SecretField` of its own — read-only, revealed,
 * with a copy accessory — so it is the same control as the field it will fill.
 * It was a `<pre>` with a button beside it, which read as a different kind of
 * thing doing the same job.
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

	// Opening the popover IS the request. Nobody presses "generate a password" to
	// be asked again, and an empty panel with a Generate button in it reads as one
	// that generated nothing.
	//
	// Guarded on there being nothing yet, so reopening the popover shows the value
	// it last minted instead of quietly replacing the one that is already in the
	// field.
	useEffect(() => {
		if (!open || candidate || error || pending) return;
		mint();
		// mint is recreated each render and depends on the same state this guards
		// on; listing it would re-run this on every keystroke in the length field.
		// eslint-disable-next-line react-hooks/exhaustive-deps
	}, [open]);

	function mint(nextPreset: Preset = preset, nextLength: number = length) {
		// Checked here as well as in the API so a mistyped length is a message
		// under the field rather than a round trip that comes back 400.
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

				{error ? <p className="text-xs text-danger-fg">{error}</p> : null}

				{candidate ? (
					<SecretField
						// Keyed by the value so each fresh candidate is a fresh field,
						// rather than one carrying the previous reveal state.
						key={candidate}
						value={candidate}
						label="the generated value"
						readOnly
						defaultRevealed
					>
						<CopyAccessory />
					</SecretField>
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
