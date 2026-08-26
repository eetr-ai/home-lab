"use client";

/**
 * How the agent's Markdown is styled.
 *
 * Its own module rather than a constant inside AgentMessage: it is a table of
 * renderers with no logic in it, and keeping it there put the component over the
 * file's line budget for reasons that had nothing to do with the component.
 *
 * Everything not named here takes the surrounding prose defaults.
 */

export const MARKDOWN_COMPONENTS = {
	a: (props: React.ComponentProps<"a">) => (
		// The agent can be wrong about a link, and answers cite things it fetched.
		// noreferrer keeps this panel's URLs out of whatever it points at.
		<a {...props} target="_blank" rel="noreferrer noopener" className="text-brand underline" />
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
	table: (props: React.ComponentProps<"table">) => (
		<div className="my-2 overflow-x-auto">
			<table {...props} className="w-full border-collapse text-xs" />
		</div>
	),
	th: (props: React.ComponentProps<"th">) => (
		<th {...props} className="border border-border px-2 py-1 text-left font-medium" />
	),
	td: (props: React.ComponentProps<"td">) => (
		<td {...props} className="border border-border px-2 py-1 align-top" />
	),
	ul: (props: React.ComponentProps<"ul">) => <ul {...props} className="my-1.5 list-disc pl-5" />,
	ol: (props: React.ComponentProps<"ol">) => <ol {...props} className="my-1.5 list-decimal pl-5" />,
	p: (props: React.ComponentProps<"p">) => <p {...props} className="my-1.5 first:mt-0 last:mb-0" />,
	h1: (props: React.ComponentProps<"h1">) => (
		<h1 {...props} className="mt-3 mb-1 text-sm font-semibold" />
	),
	h2: (props: React.ComponentProps<"h2">) => (
		<h2 {...props} className="mt-3 mb-1 text-sm font-semibold" />
	),
	h3: (props: React.ComponentProps<"h3">) => (
		<h3 {...props} className="mt-3 mb-1 text-sm font-semibold" />
	),
};
