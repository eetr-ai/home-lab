// NOTE: nothing here declares "use client". Most of these primitives are pure
// and render anywhere, but the barrel also pulls in the overlays (SidePanel,
// ConfirmDialog) and their hooks, so importing it from a Server Component fails
// on `useRef`. Client components import the barrel; Server Components import the
// one or two modules they need directly — see src/app/page.tsx.
export { cn } from "./cn";
export { Button, buttonVariants, type ButtonProps, type ButtonVariant } from "./button";
export {
	IconButton,
	type IconButtonProps,
	type IconButtonVariant,
} from "./icon-button";
export { Banner, type BannerProps, type BannerVariant } from "./banner";
export { CopyButton } from "./copy-button";
export {
	Card,
	SectionCard,
	type CardPadding,
	type CardProps,
	type SectionCardProps,
} from "./card";
export { Input, inputClass, type InputProps } from "./input";
export {
	SecretField,
	RevealAccessory,
	CopyAccessory,
	useSecretField,
	type SecretFieldProps,
	type SecretFieldContextValue,
} from "./secret-field";
export { Select, selectClass, type SelectProps } from "./select";
export { Combobox, type ComboboxProps } from "./combobox";
export { Checkbox, type CheckboxProps } from "./checkbox";
export { Label, type LabelProps } from "./label";
export { FormField, type FormFieldProps } from "./form-field";
export { Spinner, FullPageSpinner, type SpinnerProps } from "./spinner";
export {
	InlineDeleteConfirm,
	type InlineDeleteConfirmProps,
} from "./delete-confirm";
export { PageHeader, type PageHeaderProps } from "./page-header";
export { EmptyState, type EmptyStateProps } from "./empty-state";
// The overlay hooks (use-presence / use-focus-trap / use-scroll-lock) are
// implementation details of these two and are intentionally not exported.
export { SidePanel, type SidePanelProps, type SidePanelWidth } from "./side-panel";
export { ConfirmDialog, type ConfirmDialogProps } from "./confirm-dialog";
export { Popover, type PopoverProps, type PopoverWidth } from "./popover";
export {
	Table,
	THead,
	TBody,
	Th,
	Td,
	type TableProps,
	type THeadProps,
	type TBodyProps,
	type ThProps,
	type TdProps,
} from "./table";
