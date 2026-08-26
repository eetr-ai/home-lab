import type { NextConfig } from "next";

const nextConfig: NextConfig = {
	// Emit a self-contained server bundle under .next/standalone, so the runtime
	// image copies one directory instead of installing node_modules again. The
	// Dockerfile depends on this; turning it off breaks the image, not the dev
	// server, which is the kind of failure worth naming here.
	output: "standalone",
};

export default nextConfig;
