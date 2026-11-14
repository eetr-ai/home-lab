import { forwardRef, type SVGProps } from "react";

/**
 * The Redis "R", as a monochrome `currentColor` icon beside the lucide icons in
 * the nav, the same way PostgresIcon is the elephant. It is the mark from
 * redis.io's own favicon, flattened: the white glyph traced out of the red
 * square, with the square dropped so the icon takes the theme's colour like
 * everything around it.
 *
 * Traced rather than copied because redis.io publishes the mark only as a
 * raster. The path is a polygon at roughly a sixth of a viewBox unit of error,
 * which is invisible at the 16px the nav draws it. The bowl's counter is a
 * second subpath, hence the even-odd fill.
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
			<path fillRule="evenodd" clipRule="evenodd" d="M16.59 117.82L15.06 117.53 13.45 116.14 11.49 113.55 9.48 110.19 5.58 101.48 4.33 97.36 4.07 95.13 8.43 89.73 17.42 79.65 59.29 34.46 52.77 36.70 44.99 40.39 38.00 44.38 25.90 52.35 25.14 52.55 23.86 51.56 22.19 48.96 19.42 42.09 17.61 34.61 17.16 30.64 17.20 28.03 21.78 24.97 27.28 21.89 33.38 19.03 39.64 16.58 46.82 14.27 53.54 12.57 59.95 11.35 67.12 10.46 73.54 10.14 79.18 10.30 84.37 10.89 88.96 11.82 93.54 13.22 97.81 15.04 101.93 17.45 105.44 20.18 109.59 24.69 112.85 30.19 114.97 36.60 115.74 43.47 115.57 47.28 115.11 50.19 113.26 56.29 110.11 62.40 105.90 67.83 100.41 72.67 94.45 76.14 91.25 77.42 87.43 78.49 83.76 79.08 79.79 79.28 73.99 78.48 71.25 77.54 68.80 76.32 64.76 73.24 62.48 70.64 60.10 66.98 60.02 67.59 60.71 72.48 62.06 77.06 64.22 81.48 66.97 85.11 70.02 87.76 73.69 89.76 77.81 90.97 81.63 91.35 86.51 91.15 90.94 90.38 95.37 88.97 99.18 87.16 103.61 84.25 107.57 80.87 111.68 76.60 118.12 69.04 119.18 68.45 120.25 68.28 122.39 69.01 123.64 70.49 123.97 72.63 122.57 77.36 120.45 83.01 117.53 89.27 115.21 93.36 112.21 97.82 108.58 102.25 104.07 106.59 99.64 109.93 94.91 112.64 89.41 114.84 83.76 116.20 78.42 116.71 72.92 116.47 68.04 115.42 64.10 113.85 60.25 111.46 56.82 108.35 53.92 104.69 51.20 99.80 49.37 95.07 48.13 90.34 46.97 83.49 36.13 96.97 25.44 109.42 19.18 116.31 17.81 117.37 16.59 117.82ZM78.92 59.35L82.24 58.92 85.14 58.16 88.34 56.78 90.33 55.55 92.47 53.68 93.99 51.69 95.09 49.42 95.70 46.83 95.72 43.77 95.24 41.48 94.20 39.19 92.81 37.36 90.79 35.62 88.50 34.30 85.60 33.22 82.54 32.52 82.04 32.78 66.33 56.60 68.80 57.85 71.86 58.79 75.06 59.29 78.92 59.35Z" />
		</svg>
	);
});
