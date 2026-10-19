// "Show data sources": a per-browser preference, like the theme (D22, D32).
// Off by default. On, it shows where plays and track details came from: the
// Overview mix meters, History's per-play source lamps, and the badge on
// pages with provisional details. "Names only" shows regardless, since it
// explains visibly missing artwork and lengths.
const KEY = 'spotussi:show-sources';

function read() {
	try {
		return localStorage.getItem(KEY) === '1';
	} catch {
		return false;
	}
}

class SourcesPref {
	show = $state(read());

	set(on: boolean) {
		this.show = on;
		try {
			if (on) localStorage.setItem(KEY, '1');
			else localStorage.removeItem(KEY);
		} catch {
			// not persisted
		}
	}
}

export const sources = new SourcesPref();
