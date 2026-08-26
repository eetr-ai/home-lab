"use client";

/**
 * How the agent's Markdown is styled.
 *
 * Its own module rather than a constant inside AgentMessage: it is a table of
 * renderers with no logic in it, and keeping it there put the component over the
 * file's line budget for reasons that had nothing to do with the component.
 *
 * Every element CommonMark and GFM can produce is named here, which is a
 * deliberate change from styling only the common ones. There is no typography
 * plugin behind this, so an element left out does not fall back to something
 * tasteful — it falls back to the browser's stylesheet, which is Times-ish
 * margins and a rule that looks nothing like this panel. A blockquote or a
 * strikethrough is rare in an answer, and rare is exactly when nobody notices it
 * has been wrong for months.
 */

/** Header and body cells share their padding, so the columns line up. */
const CELL = "px-2.5 py-1.5 align-top";

export const MARKDOWN_COMPONENTS = {
	a: (props: React.ComponentProps<"a">) => (
		// The agent can be wrong about a link, and answers cite things it fetched.
		// noreferrer keeps this panel's URLs out of whatever it points at.
		//
		// accent-fg and not brand, which is the same near-black navy in both themes:
		// fine behind white on a button, and 1.5:1 against the dark background here,
		// which is a link nobody can see. accent-fg is the role that is defined
		// separately per theme precisely so it stays legible as text.
		<a
			{...props}
			target="_blank"
			rel="noreferrer noopener"
			className="text-accent-fg underline"
		/>
	),
	code: ({ className, children, ...props }: React.ComponentProps<"code">) => {
		// react-markdown gives a fenced block a language class and an inline span
		// none, which is the only way to tell them apart.
		if (/language-/.test(className ?? "")) {
			return (
				<code {...props} className={`${className ?? ""} block font-mono text-xs`}>
					{children}
				</code>
			);
		}
		return (
			<code {...props} className="rounded-chip bg-surface-sunken px-1 py-0.5 font-mono text-xs">
				{children}
			</code>
		);
	},
	pre: (props: React.ComponentProps<"pre">) => (
		// Overflow rather than wrap: a wrapped command is not copyable as a command.
		<pre
			{...props}
			className="my-2 overflow-x-auto rounded-card border border-border bg-surface-sunken p-2"
		/>
	),

	// ------------------------------------------------------------------ tables
	//
	// A table is the shape most worth reading well here: the agent answers "which
	// pods are restarting" with one, and a grid of hairlines makes every cell look
	// equally important. So the header carries a tint that separates it from the
	// body at a glance, rows alternate rather than being boxed, and a row lights
	// up under the pointer — which is what makes a wide row traceable across to
	// its last column.
	table: (props: React.ComponentProps<"table">) => (
		// The scroller and the frame are the same element: a rounded border on the
		// outside of a scrolling child would sit still while the content slid out
		// from under its corners.
		<div className="my-2 overflow-x-auto rounded-card border border-border">
			<table {...props} className="w-full border-collapse text-xs" />
		</div>
	),
	thead: (props: React.ComponentProps<"thead">) => (
		<thead {...props} className="bg-brand-muted text-foreground" />
	),
	th: (props: React.ComponentProps<"th">) => (
		<th {...props} className={`${CELL} border-b border-border-strong text-left font-semibold`} />
	),
	// The row rules live on tbody rather than on a `tr` renderer, because a `tr`
	// renderer cannot tell which section it is in: the header is a row too, and it
	// would have taken the striping and the hover straight over its own tint.
	tbody: (props: React.ComponentProps<"tbody">) => (
		<tbody
			{...props}
			className="[&>tr]:border-b [&>tr]:border-border [&>tr]:transition-colors [&>tr:last-child]:border-0 [&>tr:nth-child(even)]:bg-surface-sunken [&>tr:hover]:bg-surface-hover"
		/>
	),
	td: (props: React.ComponentProps<"td">) => <td {...props} className={CELL} />,

	// ------------------------------------------------------------------- lists
	//
	// A task list is a list of checkboxes, and a bullet beside each one is a
	// bullet beside a box. GFM marks both the list and its items with a class,
	// which is the only signal there is.
	ul: ({ className, ...props }: React.ComponentProps<"ul">) => {
		const tasks = /contains-task-list/.test(className ?? "");
		return <ul {...props} className={`my-1.5 ${tasks ? "list-none pl-0" : "list-disc pl-5"}`} />;
	},
	ol: (props: React.ComponentProps<"ol">) => <ol {...props} className="my-1.5 list-decimal pl-5" />,
	li: (props: React.ComponentProps<"li">) => <li {...props} className="my-0.5" />,
	input: (props: React.ComponentProps<"input">) => (
		// The only input Markdown produces is a task-list checkbox, and
		// react-markdown already ships it disabled — nothing here is a form.
		<input {...props} className="mr-1.5 align-middle accent-brand" />
	),

	// ------------------------------------------------------------------- prose
	p: (props: React.ComponentProps<"p">) => <p {...props} className="my-1.5 first:mt-0 last:mb-0" />,
	strong: (props: React.ComponentProps<"strong">) => (
		<strong {...props} className="font-semibold text-foreground" />
	),
	em: (props: React.ComponentProps<"em">) => <em {...props} className="italic" />,
	del: (props: React.ComponentProps<"del">) => (
		<del {...props} className="text-muted-foreground line-through" />
	),
	blockquote: (props: React.ComponentProps<"blockquote">) => (
		// Quoted text in an answer is nearly always something the agent read
		// somewhere else, so it is set back and dimmed rather than emphasised.
		<blockquote
			{...props}
			className="my-2 border-l-2 border-border-strong pl-3 text-muted-foreground"
		/>
	),
	hr: (props: React.ComponentProps<"hr">) => (
		<hr {...props} className="my-3 border-0 border-t border-border" />
	),
	img: (props: React.ComponentProps<"img">) => (
		// A plain <img>, not next/image: the source is whatever the answer names, and
		// next/image needs the host configured ahead of time. An alt is forced to the
		// empty string when Markdown gave none, so a screen reader skips a decorative
		// image rather than reading its URL aloud.
		// eslint-disable-next-line @next/next/no-img-element
		<img
			{...props}
			alt={props.alt ?? ""}
			className="my-2 max-w-full rounded-card border border-border"
		/>
	),

	// Headings run h1..h6 at one size on purpose. This is a 32rem column of chat,
	// not a document: what a heading marks here is a new part of one answer, and a
	// six-step ramp would have the deepest ones smaller than the text they head.
	// Weight and spacing carry the level instead.
	h1: (props: React.ComponentProps<"h1">) => (
		<h1 {...props} className="mt-3 mb-1 text-sm font-semibold" />
	),
	h2: (props: React.ComponentProps<"h2">) => (
		<h2 {...props} className="mt-3 mb-1 text-sm font-semibold" />
	),
	h3: (props: React.ComponentProps<"h3">) => (
		<h3 {...props} className="mt-3 mb-1 text-sm font-semibold" />
	),
	h4: (props: React.ComponentProps<"h4">) => (
		<h4 {...props} className="mt-2 mb-1 text-sm font-medium" />
	),
	h5: (props: React.ComponentProps<"h5">) => (
		<h5 {...props} className="mt-2 mb-1 text-sm font-medium text-muted-foreground" />
	),
	h6: (props: React.ComponentProps<"h6">) => (
		<h6 {...props} className="mt-2 mb-1 text-sm font-medium text-muted-foreground" />
	),

	// GFM footnotes: a superscript link in the text, and a section at the end that
	// arrives with its own heading and rule already rendered above.
	sup: (props: React.ComponentProps<"sup">) => (
		<sup {...props} className="text-[0.7em] leading-none" />
	),
	section: ({ className, ...props }: React.ComponentProps<"section">) => {
		if (!/footnotes/.test(className ?? "")) return <section {...props} />;
		return (
			<section
				{...props}
				className="mt-3 border-t border-border pt-2 text-xs text-muted-foreground"
			/>
		);
	},
};
