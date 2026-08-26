import { defineConfig, globalIgnores } from "eslint/config";
import nextVitals from "eslint-config-next/core-web-vitals";
import nextTs from "eslint-config-next/typescript";

const eslintConfig = defineConfig([
	...nextVitals,
	...nextTs,
	// Keep files small and focused — see docs/contributing/layer-conventions.md.
	// A warning fails the build (`eslint --max-warnings 0`), so this is a real
	// bound rather than a suggestion.
	{
		rules: {
			"max-lines": ["warn", { max: 200, skipBlankLines: true, skipComments: true }],
		},
	},
	// Tests may be longer than the code they cover: a table of cases is one
	// concern however many rows it has.
	{
		files: ["**/*.test.ts", "**/*.test.tsx"],
		rules: { "max-lines": "off" },
	},
	globalIgnores([".next/**", "out/**", "build/**", "coverage/**", "next-env.d.ts"]),
]);

export default eslintConfig;
