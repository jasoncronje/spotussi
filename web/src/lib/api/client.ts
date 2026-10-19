// Typed client for /api/v1, generated from api/openapi.yaml.
// Regenerate the types with `pnpm gen:api` after changing the spec.
//
// Middleware adds the CSRF header to unsafe requests (D13) and reports a
// lost session (401 on an authenticated call) so the app can route to login.
import createClient, { type Middleware } from 'openapi-fetch';
import type { components, paths } from './schema.js';

// Always revalidate (the API sends ETags, so this costs a 304). Without it
// the browser may reuse a heuristically cached response from something in
// front of the server, such as a DNS filter's block page with a
// Last-Modified date and no Cache-Control, for weeks.
export const api = createClient<paths>({ baseUrl: '/api/v1', cache: 'no-cache' });

export type Schemas = components['schemas'];

const UNSAFE = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);

/** Set from /auth/session (and login/setup responses). */
let csrfToken: string | null = null;
let onUnauthorized: (() => void) | null = null;

export function setCsrfToken(token: string | null) {
	csrfToken = token;
}

/** Called when an authenticated request comes back 401 (session expired or revoked). */
export function setUnauthorizedHandler(fn: () => void) {
	onUnauthorized = fn;
}

/**
 * The CSRF token for unsafe requests made outside this client (the export
 * upload uses XMLHttpRequest for upload progress).
 */
export function csrfHeader(): Record<string, string> {
	return csrfToken ? { 'X-CSRF-Token': csrfToken } : {};
}

/** Reports a 401 from a request made outside this client. */
export function reportUnauthorized() {
	onUnauthorized?.();
}

const auth: Middleware = {
	onRequest({ request }) {
		if (csrfToken && UNSAFE.has(request.method)) {
			request.headers.set('X-CSRF-Token', csrfToken);
		}
		return request;
	},
	onResponse({ request, response }) {
		// /auth/* answers 401 for a wrong password; that's not a lost session.
		if (response.status === 401 && !new URL(request.url).pathname.includes('/auth/')) {
			onUnauthorized?.();
		}
		return response;
	}
};

api.use(auth);
