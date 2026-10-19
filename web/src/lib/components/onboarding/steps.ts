// First-run steps, in order: the owner password (/setup, password mode
// only), then the Spotify guide at /welcome?step=… One meter channel each.
export const ONBOARDING = [
	{ id: 'owner', label: 'Password' },
	{ id: 'app', label: 'Spotify app' },
	{ id: 'client', label: 'Client ID' },
	{ id: 'connect', label: 'Connect' },
	{ id: 'timezone', label: 'Time zone' },
	{ id: 'history', label: 'History' }
] as const;

export type OnboardingStep = (typeof ONBOARDING)[number]['id'];
export type GuideStep = Exclude<OnboardingStep, 'owner'>;
