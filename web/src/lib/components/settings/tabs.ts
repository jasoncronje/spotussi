import { resolve } from '$app/paths';
import SlidersHorizontal from '@lucide/svelte/icons/sliders-horizontal';
import LibraryBig from '@lucide/svelte/icons/library-big';
import AudioLines from '@lucide/svelte/icons/audio-lines';
import Upload from '@lucide/svelte/icons/upload';
import KeyRound from '@lucide/svelte/icons/key-round';
import Activity from '@lucide/svelte/icons/activity';
import { rangeSearch } from '#lib/state/view.svelte.js';

// Settings tabs: each is its own route under /settings, so every group can
// be linked to directly. Old in-page anchors (#import, …) map to their tab.
export const SETTINGS_TABS = [
	{ id: 'general', label: 'General', icon: SlidersHorizontal },
	{ id: 'library', label: 'Library', icon: LibraryBig },
	{ id: 'spotify', label: 'Spotify', icon: AudioLines },
	{ id: 'import', label: 'Import', icon: Upload },
	{ id: 'tokens', label: 'API & MCP', icon: KeyRound },
	{ id: 'system', label: 'System', icon: Activity }
] as const;

export type SettingsTab = (typeof SETTINGS_TABS)[number]['id'];

/** The anchors of the single-page Settings, for links made before the tabs. */
export const LEGACY_ANCHORS: Record<string, SettingsTab> = {
	counting: 'general',
	display: 'general',
	exclusions: 'library',
	playlists: 'library',
	spotify: 'spotify',
	import: 'import',
	tokens: 'tokens',
	system: 'system',
	access: 'system'
};

/** Link to one Settings tab (the range rides along, like every section link). */
export function settingsHref(tab: SettingsTab) {
	return resolve(`/settings/${tab}` as `/settings/${SettingsTab}`) + rangeSearch();
}
