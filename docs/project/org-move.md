# Move to ussi-group (2026-11-27)

The repository moved from `jasoncronje/spotussi` to `ussi-group/spotussi`. GitHub redirects the old URLs.

## Done
- Repository transferred; issues, releases, stars and the Actions secrets came with it.
- GitHub links in the docs, site, issue templates, launch video and the settings screen now point at `ussi-group/spotussi`.
- Cloudflare Workers Builds reconnected to `ussi-group/spotussi`.

## Left
- **Container images:** packages don't move with a transfer. Existing images stay at `ghcr.io/jasoncronje/spotussi`; `release.yml` publishes the next release to `ghcr.io/ussi-group/spotussi`. After that release, switch the `ghcr.io/...` references (README, SECURITY.md, docs/install.md, docs/configuration.md, site/index.html) and the `--repo` in SECURITY.md's attestation commands, and tell users to change their image.
- `spotussi-archive` (private) is still under `jasoncronje`.
- Portfolio and second-brain mention the old path; redirects cover them.
