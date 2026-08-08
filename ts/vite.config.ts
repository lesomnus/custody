import { defineConfig } from 'vite'

/**
 * Two pages, one package.
 *
 * They are separate entries rather than separate projects because what makes
 * them two is which server they point at -- and that is a build, not a bundle.
 * Rollup gives each entry its own chunk, so `dist/index.html` is the customer
 * UI and `dist/admin/index.html` is headquarters', and a deployment serves the
 * one it means to serve.
 *
 * It is worth saying what this split is **not**. Unlike `custody` and
 * `custody-admin`, it is not a security boundary: anything a page can do, curl
 * can do, and the wall is on the server. What it buys is that the customer
 * bundle carries no transfer form for an endpoint it cannot reach, and that
 * each page has one address in it.
 */
export default defineConfig({
	build: {
		rollupOptions: {
			input: {
				customer: 'index.html',
				admin: 'admin/index.html',
			},
		},
	},
})
