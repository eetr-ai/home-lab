import { forwardRef, type SVGProps } from "react";

/**
 * The Google Drive mark, in its own colours.
 *
 * Unlike PostgresIcon this is not `currentColor`. It labels the connection to a
 * third-party service, and Google's brand guidance is to show the product mark
 * as it is rather than recoloured. That is also what makes it recognisable at a
 * glance: a monochrome triangle reads as a warning sign. Its colours are the
 * mark's own, not the panel's, which is why they are literal here rather than
 * theme tokens; on either theme they sit on the card's surface the way the mark
 * does everywhere else.
 *
 * Typed like a lucide icon so it drops in anywhere one does; size maps to
 * width and height, and className still wins.
 */
export const GoogleDriveIcon = forwardRef<
	SVGSVGElement,
	SVGProps<SVGSVGElement> & { size?: number | string }
>(function GoogleDriveIcon({ size = 24, ...rest }, ref) {
	return (
		<svg
			ref={ref}
			xmlns="http://www.w3.org/2000/svg"
			width={size}
			height={size}
			viewBox="0 0 87.3 78"
			aria-hidden="true"
			{...rest}
		>
			<path d="m6.6 66.85 3.85 6.65c.8 1.4 1.95 2.5 3.3 3.3l13.75-23.8h-27.5c0 1.55.4 3.1 1.2 4.5z" fill="#0066da" />
			<path d="m43.65 25-13.75-23.8c-1.35.8-2.5 1.9-3.3 3.3l-25.4 44a9.06 9.06 0 0 0 -1.2 4.5h27.5z" fill="#00ac47" />
			<path d="m73.55 76.8c1.35-.8 2.5-1.9 3.3-3.3l1.6-2.75 7.65-13.25c.8-1.4 1.2-2.95 1.2-4.5h-27.502l5.852 11.5z" fill="#ea4335" />
			<path d="m43.65 25 13.75-23.8c-1.35-.8-2.9-1.2-4.5-1.2h-18.5c-1.6 0-3.15.45-4.5 1.2z" fill="#00832d" />
			<path d="m59.8 53h-32.3l-13.75 23.8c1.35.8 2.9 1.2 4.5 1.2h50.8c1.6 0 3.15-.45 4.5-1.2z" fill="#2684fc" />
			<path d="m73.4 26.5-12.7-22c-.8-1.4-1.95-2.5-3.3-3.3l-13.75 23.8 16.15 28h27.45c0-1.55-.4-3.1-1.2-4.5z" fill="#ffba00" />
		</svg>
	);
});
