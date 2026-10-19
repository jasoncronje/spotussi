// Archivo (latin, width axis) before the first render. The fontsource CSS
// uses font-display: swap, so without this the app paints in the system
// fallback and every line reflows a moment later when Archivo arrives.
// Vite resolves this to the same hashed file layout.css points at, so the
// browser fetches it once; the face registered here is already loaded when
// the first frame lays out.
import latin from '@fontsource-variable/archivo/files/archivo-latin-wdth-normal.woff2?url';

let ready: Promise<unknown> | undefined;

/** Resolves when the UI face is loaded, or after `timeout` ms (slow network). */
export function fontsReady(timeout = 1500): Promise<unknown> {
	ready ??= (async () => {
		if (typeof FontFace === 'undefined') return;
		const face = new FontFace('Archivo Variable', `url(${latin}) format('woff2')`, {
			style: 'normal',
			weight: '100 900',
			stretch: '62% 125%',
			display: 'swap',
			unicodeRange:
				'U+0000-00FF,U+0131,U+0152-0153,U+02BB-02BC,U+02C6,U+02DA,U+02DC,U+0304,U+0308,U+0329,U+2000-206F,U+20AC,U+2122,U+2191,U+2193,U+2212,U+2215,U+FEFF,U+FFFD'
		});
		document.fonts.add(face);
		await face.load().catch(() => {});
	})();
	return Promise.race([ready, new Promise((r) => setTimeout(r, timeout))]);
}
