import path from "node:path";
import { defineConfig } from "vitest/config";

export default defineConfig({
	test: {
		// Node, not jsdom: this project tests logic, never markup. See
		// docs/contributing/testing.md.
		environment: "node",
		globals: true,
		include: ["src/**/*.test.ts"],
		coverage: {
			provider: "v8",
			reporter: ["text", "json-summary"],
			// src/lib is where the logic lives; components are deliberately not
			// counted, because they are deliberately not tested.
			include: ["src/lib/**/*.ts"],
		},
	},
	resolve: {
		alias: {
			"@": path.resolve(__dirname, "./src"),
		},
	},
});
