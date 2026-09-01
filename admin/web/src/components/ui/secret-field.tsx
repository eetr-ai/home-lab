"use client";

import { createContext, useContext, useState, type ReactNode } from "react";
import { Eye, EyeOff } from "lucide-react";
import { CopyButton } from "./copy-button";
import { IconButton } from "./icon-button";
import { cn } from "./cn";

/**
 * A field for a secret value, and the accessories that act on it.
 *
 * The problem this solves is that a secret value is never *just* a box: it wants
 * revealing, generating, copying, and which of those depends on where it is. The
 * first version wired each one by props at every call site, and the accessories
 * drifted apart — the reveal and generate buttons sat inside the field's border
 * while a copy button beside a generated value sat outside it, which read as two
 * different controls doing one kind of thing.
 *
 * So the field publishes a context and the accessories read it. An accessory is
 * written once, knows nothing about who is rendering it, and looks the same
 * everywhere because it is the same component in the same slot.
 *
 *   <SecretField value={password} onChange={setPassword}>
 *     <RevealAccessory />
 *     <GenerateAccessory />
 *   </SecretField>
 *
 *   <SecretField value={candidate} readOnly defaultRevealed>
 *     <CopyAccessory />
 *   </SecretField>
 *
 * Accessories that need this application's API — the generator does, it calls a
 * server action — live in app/(panel)/_components rather than here. The context
 * is the seam that lets them, without this module knowing they exist.
 */

export interface SecretFieldContextValue {
	value: string;
	setValue: (value: string) => void;
	revealed: boolean;
	setRevealed: (revealed: boolean) => void;
	/** So an accessory can say which field it belongs to in its label. */
	label: string;
	readOnly: boolean;
}

const SecretFieldContext = createContext<SecretFieldContextValue | null>(null);

/**
 * Read the field an accessory is inside.
 *
 * Throws rather than returning null: an accessory outside a SecretField is a
 * mistake with no sensible fallback, and a control that silently does nothing is
 * worse than one that fails while you are building the page.
 */
export function useSecretField(): SecretFieldContextValue {
	const field = useContext(SecretFieldContext);
	if (!field) {
		throw new Error("a secret field accessory must be rendered inside a <SecretField>");
	}
	return field;
}

export interface SecretFieldProps {
	value: string;
	/** Omitted for a read-only field, which has nothing to report. */
	onChange?: (value: string) => void;
	/** Shown in an accessory's accessible label, so several on a page differ. */
	label?: string;
	readOnly?: boolean;
	/** Start visible. For a value just generated, where dots would say nothing. */
	defaultRevealed?: boolean;
	id?: string;
	placeholder?: string;
	required?: boolean;
	className?: string;
	/** The accessories. They read the field through useSecretField. */
	children?: ReactNode;
}

/**
 * The bordered group, with the input filling it and the accessories at its end.
 *
 * The border and the focus ring belong to the group rather than to the input —
 * `focus-within` — so focusing the box lights up the whole control including
 * whatever sits beside it. That is the part that makes an accessory look like it
 * is in the field rather than next to it.
 */
export function SecretField({
	value,
	onChange,
	label = "this value",
	readOnly = false,
	defaultRevealed = false,
	id,
	placeholder,
	required,
	className,
	children,
}: SecretFieldProps) {
	const [revealed, setRevealed] = useState(defaultRevealed);

	return (
		<SecretFieldContext.Provider
			value={{
				value,
				setValue: onChange ?? (() => {}),
				revealed,
				setRevealed,
				label,
				readOnly,
			}}
		>
			<div
				className={cn(
					"flex w-full items-center rounded-control border border-border bg-background",
					"focus-within:border-brand focus-within:ring-1 focus-within:ring-brand",
					className,
				)}
			>
				<input
					id={id}
					type={revealed ? "text" : "password"}
					value={value}
					onChange={(event) => onChange?.(event.target.value)}
					readOnly={readOnly}
					placeholder={placeholder}
					required={required}
					// A generated value is not one the browser should offer to save
					// over, and a password manager filling this in would overwrite a
					// fresh one.
					autoComplete="new-password"
					spellCheck={false}
					className="w-full min-w-0 bg-transparent px-3 py-2 font-mono text-foreground placeholder:font-sans placeholder:text-muted-foreground focus:outline-none disabled:opacity-50"
				/>
				{/* empty:hidden so a field whose only accessory rendered nothing — the
				    copy button over plain HTTP does exactly that — does not keep a
				    strip of padding where a control never appeared. */}
				<div className="flex shrink-0 items-center gap-0.5 pr-1 empty:hidden">{children}</div>
			</div>
		</SecretFieldContext.Provider>
	);
}

/**
 * Show or hide the value.
 *
 * A password you cannot see is a password you cannot check you pasted correctly.
 */
export function RevealAccessory() {
	const field = useSecretField();

	return (
		<IconButton
			type="button"
			aria-label={field.revealed ? `Hide ${field.label}` : `Show ${field.label}`}
			aria-pressed={field.revealed}
			onClick={() => field.setRevealed(!field.revealed)}
		>
			{field.revealed ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
		</IconButton>
	);
}

/**
 * Copy the value.
 *
 * Renders nothing where the clipboard is absent — every plain-HTTP origin that is
 * not localhost — which is why the input beside it stays selectable.
 */
export function CopyAccessory() {
	const field = useSecretField();
	return <CopyButton text={field.value} label={`Copy ${field.label}`} />;
}
