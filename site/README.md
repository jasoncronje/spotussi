# spotussi.app

The marketing page for spotussi: plain HTML, CSS and a little JavaScript, no build step. Decisions in [D31](../docs/project/decisions.md).

- **Preview:** the `spotussi-site` launch configuration serves this folder through Portless at `https://spotussi-site.localhost`, or `python3 -m http.server -d site`.
- **Deploy:** see "Deploy" below. `.assetsignore` keeps this README out of the upload.
- **Before launch:** the GitHub links (`github.com/ussi-group/spotussi`) 404 for visitors until the repository is public (D33).
- **Hero video:** `assets/hero-reel*.mp4` and their posters are rendered with Remotion from `motion/` (see `motion/README.md`). The caption times in `main.js` match the view keyframes in `motion/src/Reel.tsx`; change them together.
- **Link previews and icons:** `og.png` (1200×630, Open Graph and Twitter/X card), `favicon.ico`, `favicon.svg` and `apple-touch-icon.png` are copies from the brand kit (`brand/social/og-1200x630.png`, `brand/web/`); rebuild them there with `brand/scripts/build.mjs` and copy them over.
- **Pictures:** The features bento and the Installation stages draw their sketches in HTML and CSS, so they have no pictures to retake. Third-party media (the album covers in `assets/covers/` and the artwork in the hero reel, from Deezer's public API) is listed in `assets/credits.json` with its rights and credited in the footer. Anything new goes there too.
- **Installation:** each stage keeps to a line or two and links its guide in `docs/`; the guides are the only install instructions. Keep the sketches illustrative and the facts in the stage text in step with `README.md` and `docs/`.

## Deploy

**Live at `https://spotussi.app/`.** It is the `spotussi` Worker: static assets only (no script runs), configured in `../wrangler.jsonc` (D35), with `spotussi.app` attached as a custom domain outside the file.

**Deploys happen through Cloudflare Workers Builds from `main`, and only that way (owner, 2026-11-20: one way to deploy every marketing site). Never run `wrangler deploy` by hand** (a Claude hook in `../.claude/` blocks it). To ship: land the change on `main` through a pull request; the push builds and deploys, and the Worker's Deployments tab shows it.

| Setting | Value |
|---|---|
| Worker | `spotussi` |
| Repository | `ussi-group/spotussi` |
| Production branch | `main` |
| Non-production branch builds | off |
| Root directory | `/` (`wrangler.jsonc` is at the repository root) |
| Build command | none (no build step) |
| Deploy command | `npx wrangler deploy` (reads `wrangler.jsonc`) |
| Build watch paths | include `site/*`, `wrangler.jsonc`; exclude `node_modules/**, .git/` (paths are from the repository root) |

No build variables or secrets. Settings live in the dashboard (Workers & Pages → `spotussi` → Settings → Builds), not in this repo. **Status: connected 2026-11-24** (Cloudflare account e00ae2a1…, the shared `Workers Builds - 2026-11-19` API token, preview builds off). The first Builds deploy was the merge of #55.

**Pull request previews are gone.** The old GitHub Actions workflow uploaded a preview at `<branch>-spotussi.<subdomain>.workers.dev` for each pull request; Workers Builds runs for `main` only, so check changes locally (the `spotussi-site` launch configuration) before merging.

The custom domain is not in `wrangler.jsonc` (no `routes`), so deploys leave it attached; `workers_dev` is off, so the site is only on `spotussi.app`.

**Roll back** a bad deploy without waiting for a new build: from the repository root, `npx wrangler@4 deployments list`, then `npx wrangler@4 rollback <version-id> -m "why"` (allowed; needs `npx wrangler@4 login` once per machine). Then fix forward on `main`; the next build deploys over the rollback. The dashboard's Deployments tab has a Rollback button too.
