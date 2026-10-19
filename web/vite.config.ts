import tailwindcss from '@tailwindcss/vite';
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// The Go server (API, mock artwork) that the dev server proxies to.
// `make dev` starts it on 8787; override with SPOTUSSI_API_URL.
const apiTarget = process.env.SPOTUSSI_API_URL ?? 'http://127.0.0.1:8787';

export default defineConfig({
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			// Static SPA (D6): every route falls back to index.html, which the Go
			// binary embeds from build/spa. build/.gitkeep stays so go:embed
			// compiles before the first UI build.
			adapter: adapter({
				pages: 'build/spa',
				assets: 'build/spa',
				fallback: 'index.html',
				precompress: true
			})
		})
	],
	server: {
		// Portless (make dev) assigns PORT/HOST; plain `pnpm dev` keeps Vite's defaults.
		port: process.env.PORT ? Number(process.env.PORT) : undefined,
		strictPort: Boolean(process.env.PORT),
		host: process.env.HOST || undefined,
		proxy: {
			'/api': { target: apiTarget, changeOrigin: true },
			'/oauth': { target: apiTarget, changeOrigin: true },
			'/_mock': { target: apiTarget, changeOrigin: true },
			'/mcp': { target: apiTarget, changeOrigin: true }
		}
	}
});
