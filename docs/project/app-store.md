# App Store submission

**Submitted 2026-11-26:** version 0.1.0, build 4, waiting for review.

Readiness review of the iPhone app (D41) against Apple's App Review Guidelines (as updated October 16, 2026) and Spotify's Developer Policy, done 2026-11-23. The TestFlight steps are in [ios/README.md](../../ios/README.md#shipping-to-testflight); this file is what the App Store submission needs on top.

## Blockers

1. **The name begins with "Spot" (risk accepted).** Spotify's [Developer Policy](https://developer.spotify.com/policy) says an app "should not begin with 'Spot' or be confusing in sound or spelling to Spotify", and its [Design & Branding Guidelines](https://developer.spotify.com/documentation/design) say the same. On the App Store this is also a 5.2.1 (intellectual property) and 4.1 (copycat) risk: Spotify can file an App Store dispute, and the app's accent is Spotify Green (D40). A self-hosted server on GitHub is a lower-profile target than a store listing with Spotify's name in the description. **Owner, 2026-11-24: keep the name** on the App Store too, accepting the risk. If Spotify objects, the fallback is a store-only name (the App Store name and display name can differ from the project name).
2. **The privacy policy URL must be live.** App Store Connect requires one, and 5.1.1(i) wants it reachable in the app. The page is `site/privacy.html` (served at `https://spotussi.app/privacy`), linked from Settings → About in the app and from the site's footer. It goes live only once Cloudflare Workers Builds is connected for the `spotussi` Worker (see [site/README.md](../../site/README.md#deploy)) and this lands on `main`. Workers Builds was connected on 2026-11-24, so merging this deploys it.

## Done in the app

- **Demo for reviewers and the curious (2.1, 4.2).** "Try the demo" on the pairing screen opens the whole app on bundled sample data, with no server and no network. Its artists, albums and playlists are made up, and since 2026-11-23 each gets generated cover art (`DemoArtwork`) instead of a blank tile. Settings → Leave demo returns to pairing.
- **No account, no sign-in (4.8, 5.1.1(v)).** Pairing uses a token from the user's own server, so account deletion and Sign in with Apple don't apply.
- **Privacy (5.1).** `PrivacyInfo.xcprivacy` declares no tracking and no collected data, plus the UserDefaults reason (CA92.1). No third-party SDKs. Purpose strings for the camera (QR scan) and local network are specific.
- **Encryption.** `ITSAppUsesNonExemptEncryption` is false (system HTTPS only).
- **No payments (3.1).** Nothing to buy; nothing links to a purchase.
- **Network (2.5).** ATS allows plain http to local names and IP addresses only (`NSAllowsLocalNetworking`); public hosts need https. Requests go through URLSession, so IPv6 works.

## Risks to watch

- **Spotify content and attribution.** The app shows Spotify metadata and artwork and offers "Open in Spotify". Spotify's guidelines ask for Spotify attribution next to its content and a link back to Spotify; the link back exists, the logo attribution doesn't. Reviewers rarely check this; Spotify might.
- **Reviewer devices (2.1).** App Review uses an iPhone 17 Pro Max and an iPad Air 11-inch (M3). The app is iPhone-only (`TARGETED_DEVICE_FAMILY` 1), so on iPad it runs in compatibility mode; check the demo there before submitting.
- **"Free" in metadata (2.3.7).** Settings → About says "Free software, AGPL-3.0." Keep that screen out of screenshots, and keep "free" out of the name, subtitle and keywords.
- **AGPL and the App Store.** Apple's terms add restrictions the AGPL forbids, which is why GPL apps have been pulled after a contributor complained. Today every commit is the owner's (plus Dependabot), so the owner can publish under other terms. Before accepting outside contributions to `ios/`, add an App Store exception to the licence or a contributor agreement; the DCO alone doesn't cover it.
- **Demo depth.** Every artist, album and track page in the demo shows the same recorded entity (one fixture per operation), with the tapped item's name only on playlists. Fine for review; a reviewer may notice.

## App Store Connect

| Field | Value |
|---|---|
| Category | Music |
| Age rating | 4+. Every questionnaire answer is No or None (no user-generated content, social features, web browsing, messaging, ads, mature themes) |
| Privacy | Data Not Collected |
| Privacy policy URL | `https://spotussi.app/privacy` |
| Support URL | `https://github.com/jasoncronje/spotussi/issues` (public once the repo is) |
| Screenshots | `brand/marketing/app-store/`, taken in demo mode. App Store Connect now takes iPhone screenshots only in the 6.1"/6.3" slot (1206×2622 among others) and rejects 1320×2868, so the uploads are those images scaled to 1206×2622 (`sips -z 2622 1206`; same aspect within 0.1%). iPhone Duo has its own slot (1398×2034 outer, 2007×2853 inner): upload `brand/marketing/app-store/duo-inner/` and `duo-outer/` as they are, no scaling |
| Availability | Free, every country except China mainland. China now needs a Security Assessment and an AIGC services declaration under App Compliance; add it once those are filed |
| Content rights | Yes: shows third-party content (Spotify metadata and artwork) with the rights to do so |
| Release | Automatic after approval |
| Sign-in required | No (the reviewer uses the demo) |

### Listing copy

- **Subtitle:** Your self-hosted music stats
- **Promotional text:** See every play since day one: top artists, tracks and albums, listening sessions and stats, straight from the server you run at home.
- **Keywords:** listening stats,music history,top artists,top tracks,self-hosted,charts,plays,sessions,habits
- **Marketing URL:** `https://spotussi.app`
- **Copyright:** 2026 Jason Cronje
- **Description:**

> Spotussi is the iPhone companion for spotussi, the open-source server you run yourself to keep statistics about your own Spotify listening. Your history stays on your server; the app reads it.
>
> • Top artists, tracks and albums for any period, ranked by plays or by time listened
> • Artist, album and track pages with play counts, first and last listens
> • Your full play history and listening sessions: when you listened, for how long, and what you played
> • Stats: time listened, plays by day, the listening clock and your averages
> • Your library and playlists, with "Open in Spotify" on everything
> • Search across everything you have ever played
>
> Pairing takes a few seconds: scan the QR code your server shows and you're in. There are no accounts, no sign-in and no tracking. The app collects no data and contains no third-party SDKs.
>
> Spotussi needs a spotussi server, which you can set up from spotussi.app. No server yet? Tap "Try the demo" on the first screen to explore every screen with sample data.
>
> Spotussi is not affiliated with or endorsed by Spotify.

Keep "Spotify" and "free" out of the name, subtitle and keywords.

### Notes for App Review

Paste into App Review Information → Notes:

> This app is a companion for a self-hosted server that the user runs on their own computer to keep statistics about their own Spotify listening. It has no accounts and no sign-in: a user pairs it by scanning a QR code shown by their server.
>
> To review without a server, tap "Try the demo" at the bottom of the first screen. Every screen then works on bundled sample data (fictional artists and generated artwork), with no network access. Settings (gear icon, top right) → Leave demo returns to the first screen.
>
> The app is read-only, collects no data and contains no third-party SDKs. The camera is used only to scan the pairing code.
