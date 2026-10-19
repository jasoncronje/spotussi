// Display preferences that come from instance settings (date format, week
// start). Formatting functions read these, so every date on screen follows
// a settings change without a reload.
import type { Schemas } from '#lib/api/client.js';

export type DateFormat = Schemas['Settings']['date_format'];

export const prefs = $state<{ dateFormat: DateFormat; weekStart: 'monday' | 'sunday' }>({
	dateFormat: 'auto',
	weekStart: 'monday'
});

export function applySettings(s: Pick<Schemas['Settings'], 'date_format' | 'week_start'>) {
	prefs.dateFormat = s.date_format;
	prefs.weekStart = s.week_start;
}
