// Whether the server answers (M8 error states). A request that can't reach
// it, or a gateway in front of it that says the server is down, marks the
// link down; the shell then shows one banner instead of every section
// failing on its own, probes /health and refetches everything once it's back.
import { ApiError } from '#lib/data/cache.js';

export const link = $state({ down: false, since: 0 });

/** Gateway answers that mean the server itself isn't there. */
const GATEWAY = new Set([502, 503, 504]);

/** True when an error means the server is unreachable, not that a request failed. */
export function unreachable(e: unknown): boolean {
	if (!(e instanceof ApiError)) return false;
	if (e.code === 'network') return true;
	// Every error the API itself sends carries a code. A 404 without one is
	// the reverse proxy saying it has no route: Traefik drops the router of
	// a stopped container, so a down server looks like 404, not 502.
	return !e.code && (GATEWAY.has(e.status) || e.status === 404);
}

/** True when a /health answer came from the server itself, whatever its status. */
export function serverAnswered(status: number, error: unknown): boolean {
	if (status >= 200 && status < 300) return true;
	const code = (error as { error?: { code?: unknown } } | undefined)?.error?.code;
	return status < 500 && typeof code === 'string';
}

export function noteFailure(e: unknown) {
	if (!unreachable(e) || link.down) return;
	link.down = true;
	link.since = Date.now();
}

export function noteSuccess() {
	if (link.down) link.down = false;
}
