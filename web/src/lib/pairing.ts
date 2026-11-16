/**
 * Pairing the iPhone app (D41). The app takes one link holding the address
 * it should call and a read-only API token:
 *
 *   spotussi://pair?url=<base URL>&token=<spt_…>
 *
 * `url` is the instance's origin, with no path. The app adds `/api/v1`.
 */

export function pairLink(origin: string, secret: string): string {
	const q = new URLSearchParams({ url: origin, token: secret });
	return `spotussi://pair?${q}`;
}

/** An address that only the computer showing the page can reach. */
export function isLoopback(hostname: string): boolean {
	const h = hostname.toLowerCase().replace(/^\[|\]$/g, '');
	return h === 'localhost' || h.endsWith('.localhost') || h === '::1' || /^127\./.test(h);
}

/**
 * A QR code as one SVG path on a `size` × `size` grid, with the four-module
 * quiet zone scanners expect. The encoder loads only when a code is drawn.
 */
export async function qrPath(text: string): Promise<{ size: number; d: string }> {
	const { encode } = await import('uqr');
	const quiet = 4;
	const { data, size } = encode(text, { ecc: 'M', border: 0 });
	let d = '';
	for (let y = 0; y < size; y++) {
		let x = 0;
		while (x < size) {
			if (!data[y][x]) {
				x++;
				continue;
			}
			let run = 1;
			while (x + run < size && data[y][x + run]) run++;
			d += `M${x + quiet} ${y + quiet}h${run}v1h-${run}z`;
			x += run;
		}
	}
	return { size: size + quiet * 2, d };
}
