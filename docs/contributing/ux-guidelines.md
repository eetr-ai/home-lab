# UX guidelines

Conventions for `admin/web`. The stack is Next.js, React, Tailwind, and
`lucide-react` icons — nothing else. These rules are adapted from the eetr-auth
admin dashboard, which is where the panel's look and its component library come
from.

## The component library

Reusable primitives live in `src/components/ui` and are imported through the
barrel (`@/components/ui`). Reach for them instead of retyping the class strings
below; the strings remain the spec, but the primitives are how it is applied.

`Button` · `IconButton` · `Banner` · `Card` · `SectionCard` · `PageHeader` ·
`Table`/`THead`/`TBody`/`Th`/`Td` · `Input` · `Select` · `Label` · `FormField` ·
`Spinner`/`FullPageSpinner` · `InlineDeleteConfirm` · `SidePanel` ·
`ConfirmDialog` · `EmptyState`.

Nothing in `components/ui` declares `"use client"`. Every consumer is a
`_components/` child of a client page, so a Server Component must not import the
barrel.

Page-specific components are colocated in a `_components/` folder next to the
route's `page.tsx`. **State stays in the page**; `_components/` children are
presentational — props in, callbacks out. Purely ephemeral UI state, such as which
row is currently asking for delete confirmation, may stay local to the child.

## Theme

`src/app/theme.css` holds two tiers:

1. **Palette** — raw values (`--gray-*`, `--brand-*`, `--red-*`). Referenced only
   by tier 2.
2. **Roles** — semantic names. **The only thing components may reference.**

Roles cover surfaces (`--background`, `--surface`, `--surface-sunken`,
`--surface-hover`), text (`--foreground`, `--muted-foreground`), edges
(`--border`, `--border-strong`), brand, status (`--danger-*`, `--success-*`,
`--warning-*`, `--accent-*`), and `--scrim`. Adding a theme is one more selector
block remapping tier 2; no component changes.

Because roles flip on their own, **component code carries no `dark:` variants at
all.** Write `bg-danger-bg text-danger-fg`, not a light class plus a `dark:`
counterpart. The old rule — write the light class, repeat it as a `dark:` twin,
remember to carry `hover:` across too — is a standing source of dark-mode bugs,
because the two halves drift.

A role defined in `:root` but missing from `.dark` silently keeps its light value
in dark mode. Every role needs every theme, including the
`@media (prefers-color-scheme: dark)` fallback that covers the moment before the
inline theme script runs.

`scripts/check-theme.mjs` runs as part of `npm run lint` and fails the build on
raw Tailwind color ramps, `rounded-xl`, or `border-brand-muted` outside
`theme.css`. Without it these creep back within a few changes, and the nested
border look the visual system exists to prevent comes back with them.

## Visual system

**Depth comes from surface, not outline.** A boundary gets exactly one edge, drawn
by the container. Children must not draw their own: **never nest a bordered
container inside another bordered container**, and separate list rows with
`divide-y divide-border` rather than a border each.

A card does legitimately pair `border-border` with `bg-surface`: in light mode the
surface and the background are the same white, so the hairline is what makes the
card visible at all, while in dark mode the raised surface does most of the work.

Radius scale, named for intent so Tailwind's own `rounded-sm|md|lg` stays free:

| Token | Size | Use |
| --- | --- | --- |
| `rounded-chip` | 4px | badges, tags, checkboxes |
| `rounded-control` | 6px | inputs, selects |
| `rounded-card` | 8px | cards, panels, tables |

Buttons are `rounded-full`. Never sharp-cornered buttons.

Spacing, owned by the primitives rather than remembered per page: page gutter
`p-6`, section gap `gap-6`, card padding `p-4` dense or `p-6` for forms, table
cell `px-4 py-2.5`, control gap `gap-2`.

## Destructive actions

**Never use `window.confirm`, `window.alert`, or `window.prompt`.** Confirmation
is inline, in the same row or card as the action that triggered it.

The page holds two pieces of state — the id of the row asking for confirmation,
and the id of the row whose request is in flight:

```ts
confirmingDeleteId: string | null;
deletingId: string | null;
```

First click on the trash icon sets `confirmingDeleteId` and sends no request.
Second click runs the mutation, tracking `deletingId` and showing a spinner on the
confirm button. Cancel or success clears it. **While a row is confirming, its
other actions are hidden** so there is exactly one decision to make. Render it with
`InlineDeleteConfirm`.

For a full-page destructive action, use the same logic with the confirmation as an
inline card above the action area — still not a modal.

The one exception is the **unsaved-changes guard** when dismissing a `SidePanel`,
which uses `ConfirmDialog`, because what is being confirmed is the dismissal
itself and the surface an inline confirmation would attach to is the thing going
away. **Deleting a record is never a dialog.**

## Buttons and banners

| Variant | Usage |
| --- | --- |
| Primary | the main call to action on a page or form |
| Secondary / ghost | neutral actions, dismissals |
| Destructive confirm | the "yes, do it" in a confirmation |
| Icon-only | per-row actions; **always** `aria-label` |

Errors and successes are **inline banners inside the section they relate to** —
not toasts, not modals. No toast library. Clear the message when the user starts a
new attempt at the same action, so a stale error does not linger.

## Directory surfaces

Every list page — PostgreSQL databases, Mongo users, Kubernetes workloads — is a
*directory surface*, and they must stay identical to each other. This is the
contract to review a change against:

1. **Page header** — `PageHeader` with icon, title, and a right-aligned
   `<Button icon={Plus}>`. No second "Manage X" card; the page title is the heading.
2. **Toolbar** — filter controls only.
3. **Table** — one bordered `rounded-card` container that owns the edge; rows
   separated by `divide-y`; `thead` on `--surface-sunken`.
4. **Row actions** — `IconButton` only, in a fixed order: surface-specific icons
   first, then `Pencil`, then `Trash2`. No pills, no "View" link.
5. **Clicking a row opens its edit panel.** The actions cell calls
   `stopPropagation`, so a row action never also opens the panel. Keep the
   `Pencil` anyway: a `<tr>` cannot carry button semantics cleanly, so the icon
   button stays the keyboard-reachable, labelled affordance.
6. **Empty versus filtered** — `EmptyState` with the header's CTA when the
   collection is genuinely empty; a plain muted line when filters merely exclude
   everything. Different problems, different fixes.
7. **Create and edit** — one `SidePanel`, titled "New X" / "Edit X".
8. **Errors** — page-level `Banner` for list errors, in-panel `Banner` for save
   errors, never both at once: a page-level banner is invisible behind the scrim.
9. **State** — `panelOpen`, `editingId`, `draft`, `baseline`, `saving`,
   `confirmingDeleteId`, `deletingId`.

A new directory surface should need no new layout class strings. If it does, the
primitive is wrong — fix the primitive.

## Section tabs

Each section (PostgreSQL, MongoDB, Kubernetes) carries tabs, and they are
**route segments, not local state**. The section `layout.tsx` owns the tab strip so
it cannot go missing from a page added later, each tab is its own segment, and the
active tab is the **longest matching href** — otherwise the section root lights up
on every page beneath it.

URL-driven tabs are bookmarkable and survive a reload, which is what you want when
you are sending someone a link to a specific view. Use local state for a tab only
when the surrounding view already owns query parameters that the tab would make
ambiguous; say so in a comment when you do.

Tab strips carry proper semantics: `role="tablist"`, `role="tab"`,
`aria-selected`, `aria-controls`, and a roving `tabIndex` so the whole strip is one
tab stop with arrow-key movement.

## Overlays

`SidePanel` is for **multi-field** create and edit forms. A single-field entity
keeps a compact inline add-row; a full-screen overlay to capture one text input
costs more screen than it saves.

**Controlled, always.** The consumer owns `open`. `onRequestClose` fires from the
X, the scrim, and Escape, and the panel never closes itself — that is exactly what
lets the dirty guard interpose.

Accessibility contract: `role="dialog"`, `aria-modal="true"`, `aria-labelledby`
the title, focus trapped, initial focus on `[data-autofocus]`, focus restored to
the trigger on close, and body scroll locked with scrollbar-gutter compensation so
the page does not shift.

An animated panel is a transformed element, so `position: fixed` **inside its
children** resolves against the panel rather than the viewport. A nested overlay
renders into its own portal as a sibling of the panel, never inside its children.

Never unmount an overlay directly on the open flag — that skips the exit
animation. Unmount on a timer, because under `prefers-reduced-motion` no
`animationend` ever fires.

**Dirty guard.** Capture a baseline when the panel opens and compare the
*persisted projection* — trim text, sort unordered id arrays — so reformatting and
checkbox order do not read as edits. Closing must not reset the draft: the panel
keeps rendering its children while it animates out, so clearing them slides out an
empty form.

## Loading states

Use `Loader2` with `animate-spin`, sized to context. Keep a loading button mounted
and swap its icon rather than replacing it with a bare spinner. Track per-row
loading with an id, not a boolean, so concurrent actions on different rows stay
independent.

**`FullPageSpinner` is for the first load only.** A refetch after a mutation must
not unmount the page — pass a `silent` flag so the section and any open overlay
stay mounted, and let the acting control show its own in-flight state.

## Icons

From `lucide-react`, matching the established vocabulary: `Pencil` edit, `Trash2`
delete, `Check` confirm, `X` cancel, `Loader2` loading, `Plus` create, `Database`
databases, `Users` roles and users, `Boxes` workloads, `KeyRound` credentials,
`Settings` configuration.

Inline icons in flow text `h-3.5 w-3.5`; row actions `h-4 w-4`; section headings
`h-5 w-5`.

## State management

Small components use `useState`. Components owning non-trivial or shared state use
a reducer built with
[`@eetr/react-reducer-utils`](https://www.npmjs.com/package/@eetr/react-reducer-utils):
define an action enum, a typed flat state, an `initialState`, and a reducer, then

```ts
const { Provider, useContextAccessors } =
  bootstrapProvider<State, ReducerAction<ActionType>>(reducer, initialState);
```

Action payloads go on `ReducerAction`'s `data` field. Adding an interaction means:
an entry in the enum, a field in the state interface and `initialState`, a reducer
case, and the field destructured in the component. No ad-hoc global state outside
this pattern — and the reducer, being a pure function, is exactly the kind of thing
[testing.md](testing.md) says to test.

## What to avoid

- `window.confirm`, `window.alert`, `window.prompt`.
- Toast libraries, and third-party dialog, drawer, or overlay libraries (Radix,
  Headless UI, framer-motion). Status messaging is inline banners; overlays are
  the first-party `SidePanel` and `ConfirmDialog`.
- New third-party UI component libraries. Tailwind plus `lucide-react` is the
  stack.
- Raw Tailwind color ramps, `rounded-xl`, and `border-brand-muted` as a default
  border — all three fail lint.
- Emojis in UI copy.
- Sharp-cornered buttons, and bright primaries outside the brand tokens.
- Nesting a bordered container inside another bordered container.
