// Theme: system (default), light or dark. A per-browser preference (D22),
// not a server setting: app.html applies it before first paint; this keeps
// it in sync afterwards. Chosen from the header toggle or Settings.
export type Theme = 'system' | 'light' | 'dark';

const KEY = 'spotussi:theme';

function read(): Theme {
	try {
		const t = localStorage.getItem(KEY);
		return t === 'light' || t === 'dark' ? t : 'system';
	} catch {
		return 'system';
	}
}

const media =
	typeof matchMedia === 'function' ? matchMedia('(prefers-color-scheme: dark)') : undefined;

class ThemeState {
	choice = $state<Theme>(read());
	#systemDark = $state(media?.matches ?? false);

	constructor() {
		media?.addEventListener('change', (e) => {
			this.#systemDark = e.matches;
			this.#apply();
		});
	}

	get dark() {
		return this.choice === 'dark' || (this.choice === 'system' && this.#systemDark);
	}

	set(t: Theme) {
		this.choice = t;
		try {
			if (t === 'system') localStorage.removeItem(KEY);
			else localStorage.setItem(KEY, t);
		} catch {
			// not persisted
		}
		this.#apply();
	}

	#apply() {
		document.documentElement.classList.toggle('dark', this.dark);
	}
}

export const theme = new ThemeState();
