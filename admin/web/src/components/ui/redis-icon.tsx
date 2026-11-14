import { forwardRef, type SVGProps } from "react";

/**
 * The Redis "R", as a monochrome `currentColor` icon beside the lucide icons in
 * the nav, the same way PostgresIcon is the elephant. It is the mark from
 * redis.io's own favicon, flattened: the white glyph traced out of the red
 * square, with the square dropped so the icon takes the theme's colour like
 * everything around it.
 *
 * Traced rather than copied because redis.io publishes the mark only as a
 * raster: potrace over the favicon's thresholded glyph, then flattened to
 * absolute coordinates in this 128-unit viewBox. The bowl's counter is a second
 * subpath, hence the even-odd fill.
 *
 * Typed to accept the SVG props a lucide icon does (className, size), so it
 * drops in anywhere a lucide icon does — the nav item, the page header. size
 * maps to width/height; className still wins, which is how `h-4 w-4` sizes it.
 */
export const RedisIcon = forwardRef<
	SVGSVGElement,
	SVGProps<SVGSVGElement> & { size?: number | string }
>(function RedisIcon({ size = 24, ...rest }, ref) {
	return (
		<svg
			ref={ref}
			xmlns="http://www.w3.org/2000/svg"
			width={size}
			height={size}
			viewBox="0 0 128 128"
			fill="currentColor"
			aria-hidden="true"
			{...rest}
		>
			<path fillRule="evenodd" clipRule="evenodd" d="M71 10.12C51.71 11.37 33.61 17.09 19.44 26.40L17.09 27.94L17.18 30.09C17.43 35.57 19.18 42.46 21.65 47.68C23.01 50.55 24.45 52.44 25.27 52.44C25.45 52.44 26.70 51.68 28.05 50.75C39.41 42.91 48.84 37.80 57.34 34.90C59.37 34.21 59.58 34.16 59.31 34.47C58.70 35.17 52.84 41.46 44.20 50.69C14.56 82.41 4 94.13 4.02 95.36C4.03 96.05 4.64 98.49 5.31 100.47C8.07 108.59 13.03 116.83 15.61 117.61C16.91 118 18.31 117.20 20.48 114.80C29.37 105.03 39.31 93.24 45.90 84.65C47.10 83.08 47.12 83.06 47.16 83.58C47.25 84.94 47.71 88.10 48.05 89.75C51.21 105.39 60.22 114.82 73.50 116.39C75.44 116.62 81.07 116.48 83.13 116.16C100.41 113.41 114.10 100.37 121.77 79.30C123.30 75.10 124 72.55 123.89 71.58C123.65 69.24 121.41 67.70 119.15 68.33C118.37 68.54 117.39 69.53 114.40 73.19C111.72 76.47 106.41 81.92 104.29 83.57C97.33 88.98 90.01 91.43 81.54 91.15C75.40 90.95 70.95 89.08 67.09 85.06C63.17 80.97 60.47 74.44 60.07 68.02L59.98 66.56L60.54 67.55C66.37 77.75 76.87 81.43 89.69 77.75C105.58 73.19 117 56.93 115.56 40.92C114.02 23.80 101.36 12.57 81.14 10.41C79.08 10.18 72.73 10 71 10.12ZM83.96 32.68C91.41 34.42 95.47 38.54 95.67 44.58C95.80 48.57 94.83 51.05 91.97 53.89C86.89 58.97 77.54 60.73 69.70 58.06C68.43 57.63 66.27 56.64 66.27 56.50C66.27 56.46 68.01 53.83 70.13 50.66C72.26 47.48 75.83 42.10 78.09 38.66C82.67 31.69 82.12 32.26 83.96 32.68Z" />
		</svg>
	);
});
