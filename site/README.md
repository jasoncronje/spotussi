# spotussi.app

The marketing page for spotussi: plain HTML, CSS and a little JavaScript, no build step. Decisions in [D31](../docs/project/decisions.md).

- **Preview:** the `spotussi-site` launch configuration serves this folder through Portless at `https://spotussi-site.localhost`, or `python3 -m http.server -d site`.
- **Deploy:** Cloudflare, as a static-assets Worker configured in `../wrangler.jsonc` (D35). `.github/workflows/site.yml` deploys production at `https://spotussi.app/` on every push to `main` that touches `site/`, and uploads a preview at `<branch>-spotussi.<subdomain>.workers.dev` for pull requests. By hand: `npx wrangler deploy` from the repository root. `.assetsignore` keeps this README out of the upload.
- **Before launch:** the GitHub links (`github.com/jasoncronje/spotussi`) 404 for visitors until the repository is public (D33).
- **Hero video:** `assets/hero-reel*.mp4` and their posters are rendered with Remotion from `motion/` (see `motion/README.md`). The caption times in `main.js` match the view keyframes in `motion/src/Reel.tsx`; change them together.
- **Link previews and icons:** `og.png` (1200×630, Open Graph and Twitter/X card), `favicon.ico`, `favicon.svg` and `apple-touch-icon.png` are copies from the brand kit (`brand/social/og-1200x630.png`, `brand/web/`); rebuild them there with `brand/scripts/build.mjs` and copy them over.
- **Pictures:** The features bento and the Installation stages draw their sketches in HTML and CSS, so they have no pictures to retake. Third-party media (the album covers in `assets/covers/` and the artwork in the hero reel, from Deezer's public API) is listed in `assets/credits.json` with its rights and credited in the footer. Anything new goes there too.
- **Installation:** each stage keeps to a line or two and links its guide in `docs/`; the guides are the only install instructions. Keep the sketches illustrative and the facts in the stage text in step with `README.md` and `docs/`.
