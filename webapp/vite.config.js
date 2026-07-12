import tailwindcss from "@tailwindcss/vite";
import preact from '@preact/preset-vite';
import { defineConfig } from "vite";

export default defineConfig({
	plugins: [tailwindcss(), preact()],
	server: {
		cors: {
			origin: "http://isgate.isgate-test.localhost:1443",
		},
	},
	build: {
		manifest: true,
		rolldownOptions: {
			input: [
				"src/index.tsx",
			],
		},
	},
});
