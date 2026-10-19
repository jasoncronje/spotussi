// Who the browser is (D13): auth mode, signed in or not, first-run setup
// pending, and the CSRF token for unsafe requests. Loaded once before the
// first route renders (root +layout.ts) and refreshed after sign-in/out.
import { api, setCsrfToken, type Schemas } from '#lib/api/client.js';
import { ApiError } from '#lib/data/cache.js';

export type AuthSession = Schemas['AuthSession'];

export const session = $state<{ current: AuthSession | null }>({ current: null });

let pending: Promise<AuthSession> | null = null;

export function setSession(s: AuthSession) {
	session.current = s;
	setCsrfToken(s.csrf_token);
}

/** Fetches /auth/session (shared while in flight). */
export function loadSession(force = false): Promise<AuthSession> {
	if (session.current && !force) return Promise.resolve(session.current);
	if (pending && !force) return pending;
	pending = api
		.GET('/auth/session')
		.then(({ data, error, response }) => {
			if (!data) {
				const e = (error as Schemas['Error'] | undefined)?.error;
				throw new ApiError(
					e?.message ?? 'The server could not be reached.',
					response.status,
					e?.code
				);
			}
			setSession(data);
			return data;
		})
		.finally(() => (pending = null));
	return pending;
}

/** Routes that render without the app shell and without a session. */
export const BARE_ROUTES: readonly string[] = ['/login', '/setup'];

/** Where a signed-out visitor goes, or null to stay. */
export function authRedirect(s: AuthSession, pathname: string, search: string): string | null {
	const bare = BARE_ROUTES.includes(pathname);
	if (s.setup_required) return pathname === '/setup' ? null : '/setup';
	if (!s.authenticated) {
		if (pathname === '/login') return null;
		const next =
			pathname === '/' && !search ? '' : `?next=${encodeURIComponent(pathname + search)}`;
		return '/login' + next;
	}
	if (bare) return '/';
	return null;
}
