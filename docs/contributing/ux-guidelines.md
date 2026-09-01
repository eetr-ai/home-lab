# UX guidelines

Conventions for `admin/web`. The stack is Next.js, React, Tailwind, and
`lucide-react` icons — nothing else. These rules are adapted from the eetr-auth
admin dashboard, which is where the panel's look and its component library come
from.

## The component library

Reusable primitives live in `src/components/ui`. Reach for them instead of
retyping the class strings below; the strings remain the spec, but the primitives
are how it is applied.

`Button` · `IconButton` · `Banner` · `Card` · `SectionCard` · `PageHeader` ·
`Table`/`THead`/`TBody`/`Th`/`Td` · `Input` · `Select` · `Label` · `FormField` ·
`Spinner`/`FullPageSpinner` · `InlineDeleteConfirm` · `SidePanel` ·
`ConfirmDialog` · `Popover` · `CopyButton` · `EmptyState`.

`CopyButton` renders **nothing** where `navigator.clipboard` is absent, which is
every plain-HTTP origin that is not localhost — a real way to reach this panel.
Put the text it copies somewhere selectable, because selecting it is what still
works there. It reads the clipboard's existence through `useSyncExternalStore`
with a server snapshot of `false`; consulting `navigator` during render is a
hydration mismatch over a button.

**How you import one depends on what is importing it**, and there is exactly one
rule:

- A **Client Component** imports the barrel: `import { Button, Td } from
  "@/components/ui"`.
- A **Server Component** imports the module: `import { PageHeader } from
  "@/components/ui/page-header"`.

Nothing in `components/ui` declares `"use client"`, and most of the primitives
render anywhere. The barrel is the problem, not the primitives: it also pulls in
the overlays (`SidePanel`, `ConfirmDialog`) and their hooks, so importing it from
a Server Component fails on `useRef` — for a component the page never even
rendered.

The same distinction is why a *shared* presentational component that a Server
Component renders — `Directory`, say — must not be marked `"use client"` either.
See [layer-conventions.md](layer-conventions.md) for what goes wrong when it is.

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
5. **Clicking a row opens its edit panel, or navigates — where the row leads
   anywhere at all.** The whole row, not the one cell that happens to hold a
   link: an underline on a single word is invisible until the pointer is already
   on it, so the row reads as inert and the way in gets missed. Use
   `InteractiveRow`, which supplies the pointer cursor, the hover background, and
   the `group` class that lets the naming cell underline itself on
   `group-hover`.

   The actions cell calls `stopPropagation` — `stopRowActivation` beside the
   component — so a row action never also opens the panel or navigates. Keep the
   `Pencil`, or the `<Link>` on the name: a `<tr>` cannot carry button semantics
   cleanly, so that stays the keyboard-reachable, labelled affordance and the row
   click is a convenience layered over it.

   Several surfaces here have no edit at all, because the thing behind them has
   no update operation: a PostgreSQL database is created and dropped, never
   altered through this panel, and the same goes for roles, collections, and
   Mongo users. Those surfaces carry a delete action and nothing else, and their
   rows are not clickable — a row that looks interactive and does nothing is
   worse than one that plainly is not.
6. **Empty versus filtered** — `EmptyState` with the header's CTA when the
   collection is genuinely empty; a plain muted line when filters merely exclude
   everything. Different problems, different fixes.
7. **Create and edit** — one `SidePanel`, titled "New X" / "Edit X".
8. **Errors** — page-level `Banner` for list errors, in-panel `Banner` for save
   errors, never both at once: a page-level banner is invisible behind the scrim.
9. **State** — `panelOpen`, `editingId`, `draft`, `baseline`, `saving`,
   `confirmingDeleteId`, `deletingId`. In practice the last three are not written
   out per surface: `CreatePanel` owns the saving and unsaved-changes state, and
   `useRowDelete` owns the confirm/delete pair. Both keep that state in the
   parent so only one row can be asking for confirmation at a time.

10. **Scoped lists put the scope in the query string.** A list that has no
    meaning without a database or a namespace — Mongo collections, PostgreSQL
    extensions, anything under Kubernetes — reads its scope from the URL through
    `ScopePicker`, not from component state. The scope is part of what the page
    is showing, so it should be linkable, survive a reload, and step through the
    back button. A scope named in the URL that no longer exists falls back to the
    first available one rather than erroring: the link is stale, not wrong.

    Pass `allLabel` only where the unfiltered view is meaningful — the Helm
    dashboard is, a collections list is not, and there being made to choose is
    the point. Selecting it removes the parameter rather than setting it empty,
    so the unfiltered view has one address instead of two.

11. **A detail page opens with a `BackLink` to the list it came from**, carrying
    the scope it was viewed under. The browser's back button only helps somebody
    who arrived by clicking: a detail page reached from a bookmark, a link
    somebody pasted, or the assistant's `navigate_to` is otherwise a dead end
    with no way to see what else there is.

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

A route-segment tab strip is **navigation, not an ARIA tablist**. It is a `<nav>`
of links carrying `aria-current="page"`, and it must not use `role="tab"` /
`role="tablist"` / `aria-selected`.

This guide said the opposite until the strip was built, and the earlier advice was
wrong. ARIA tabs describe panels that live in the same document and are swapped
without navigating: `aria-controls` has to name an element that is present, and a
roving `tabIndex` makes sense because the panels are siblings. Neither holds here
— activating one of these replaces the document. Announcing them as tabs promises
a screen-reader user a widget that does not behave like one, while `aria-current`
says exactly what is true: this link is the page you are on.

Local-state tabs, if a section ever needs them, *are* an ARIA tablist and should
carry the full semantics.

## The assistant drawer

The one surface here that is neither a page nor an overlay. It is a **third column
of the signed-in shell's flex row** (`src/app/(panel)/layout.tsx`), so opening it
narrows the page rather than covering it, and the page beside it stays live and
clickable.

That is not a styling preference. The agent navigates — asking it to take you to a
workload and watching the page change beside the answer that caused it is the
reason the drawer is in the shell at all — and a surface that covers the page
cannot show you that.

So it is **not** a `SidePanel` and must not be rebuilt as one. No portal, no
scrim, no focus trap, no scroll lock, and deliberately not `role="dialog"
aria-modal="true"`: every one of those tells a screen-reader user the page behind
is inert, and it is not. Escape does not close it either, for the same reason. It
carries `aria-label` and nothing else, and it is `inert` while collapsed — the
width animates, so the content stays laid out, and a zero-width column full of
focusable controls would otherwise be a tab stop into nothing.

It stays mounted once opened and collapses to `w-0`, which is what lets an answer
in flight finish arriving while it is closed.

**`react-markdown` and `remark-gfm` are a deliberate exception** to the rule
below about third-party libraries. They are a renderer, not a component library:
nothing about the panel's look comes from them, every element they emit is
styled by `src/components/agent/markdown.tsx` with the same role tokens as
everything else, and `react-markdown` builds React elements rather than HTML — so
nothing reaches `dangerouslySetInnerHTML`. That last part is the point rather
than a bonus, because the agent's answers are generated partly from text other
people wrote: pod logs, event messages, whatever `curl` fetched.

## The values editor

`src/components/editor/yaml-editor.tsx` wraps CodeMirror 6, and **`codemirror`
plus `@codemirror/lang-yaml` are the second deliberate exception** to the rule
below about third-party libraries.

The reason is specific and does not generalise. A chart's values file is a
document an operator writes, and YAML is indentation-sensitive: in a bare
textarea a misplaced space is invisible until the API rejects the whole file,
and the operator is left comparing two columns of whitespace by eye. Line
numbers, folding, bracket matching and a tab key that indents instead of moving
focus are what make it an editor rather than a box. It is also, unlike a
component library, invisible: every colour comes from `theme.css` role tokens via
`yaml-editor-theme.ts`, so it follows light and dark with the rest of the panel
and no palette lives in TypeScript.

What it is *not* is a general licence to reach for an editor. It renders one
kind of document on two surfaces, and anything else that wants rich text should
be a `textarea` until there is an argument this specific.

There is no JavaScript YAML parser anywhere in the panel, and there should not
be. The editor sends the document as a string; the API parses it with
`sigs.k8s.io/yaml`, which is the same parser Helm will use, and a syntax error
comes back as a 400 naming the line. A second parser here would eventually
disagree with that one, and the disagreement would be the panel accepting
something the cluster then refused.

## Overlays

There are three overlays and the line between them is worth stating, because the
next one has to land somewhere.

| | For | Modal? |
| --- | --- | --- |
| `SidePanel` | filling in a multi-field form | yes — scrim, scroll lock |
| `ConfirmDialog` | stopping to answer one question | yes |
| `Popover` | *looking something up* without leaving the page | no |

`Popover` is anchored to the control that opened it and is deliberately not
modal: no scrim, no scroll lock, closes on Escape or a click outside. Use it for
secondary detail that would push the primary content below the fold if it lived
inline, and that you dismiss the moment you have read it — a deployment's version
history is the case it was built for. If the thing behind it stops being the
subject while it is open, it wanted to be a `SidePanel`.

It portals to the body and positions itself with fixed coordinates measured from
its anchor, so it escapes the `overflow: hidden` of any card it opens from. That
costs a reposition on scroll and resize, which it does for you.

`SidePanel` is for **multi-field** create and edit forms. A single-field entity
keeps a compact inline add-row; a full-screen overlay to capture one text input
costs more screen than it saves.

A `Popover` **may** be opened from inside a `SidePanel` — the credential
generator is — and it works because the popover portals to the body rather than
nesting inside the panel's containing block. Both run `useFocusTrap`, so check by
hand that Escape closes the popover and leaves the panel open. Do not build a
bespoke overlay to avoid the question.

## Secret values

Anywhere an operator would type a password, use `SecretInput`. It is a password
field with reveal and a generator popover, and it lives in
`app/(panel)/_components` rather than `components/ui` because it calls a server
action — the primitives it is built from know nothing about this API, and keeping
them that way is what makes them reusable.

The generator itself is in Go, `admin/api/internal/secretgen`, reached through
`app/actions/tools.ts`. It is not in the browser, and the reason is that the
assistant needs the same generator: two implementations of rejection sampling
would agree until they did not.

**Never show a stored secret's value.** The API has no route that returns one, by
construction — a listing carries key names — and a screen that appears to reveal
one is a screen that is lying or a route that should not exist. Where an operator
asks for a value they did not keep, the answer is to rotate it.

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
  the first-party `SidePanel`, `ConfirmDialog` and `Popover`.
- New third-party UI component libraries. Tailwind plus `lucide-react` is the
  stack. There are exactly two exceptions, each argued where it is used:
  `react-markdown` and `remark-gfm` in the [assistant
  drawer](#the-assistant-drawer), and CodeMirror 6 in [the values
  editor](#the-values-editor). Both are renderers rather than component
  libraries, and neither contributes anything to how the panel looks.
- Raw Tailwind color ramps, `rounded-xl`, and `border-brand-muted` as a default
  border — all three fail lint.
- Emojis in UI copy.
- Sharp-cornered buttons, and bright primaries outside the brand tokens.
- Nesting a bordered container inside another bordered container.
