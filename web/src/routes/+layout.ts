// Client-only SPA: no server rendering, no prerendering (adapter-static
// writes the fallback page that the Go server returns for every route).
import { redirect } from '@sveltejs/kit';
import { authRedirect, loadSession } from '#lib/state/session.svelte.js';
import type { LayoutLoad } from './$types';

export const ssr = false;
export const prerender = false;

// Every navigation checks the (cached) session, so signed-out visitors land
// on login or first-run setup instead of a page full of 401s.
export const load: LayoutLoad = async ({ url }) => {
	let target: string | null = null;
	try {
		const s = await loadSession();
		target = authRedirect(s, url.pathname, url.search);
	} catch {
		// Server unreachable: let the page render and show its own errors.
	}
	if (target && target !== url.pathname + url.search) redirect(307, target);
};
