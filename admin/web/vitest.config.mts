import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vitest/config";

// Not __dirname: this file is an ES module, where that binding does not exist.
// It happens to work while Vite pre-bundles the config, which is not a promise.
const configDir = fileURLToPath(new URL(".", import.meta.url));

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
			"@": path.resolve(configDir, "src"),
		},
	},
});
