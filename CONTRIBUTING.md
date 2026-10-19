# Contributing to spotussi

Thanks for helping. spotussi is a small project with one maintainer, so a short issue before a large change saves everyone time. Bug fixes and documentation fixes are welcome straight away.

The design record is in [docs/project/decisions.md](docs/project/decisions.md) (D1, D2, …) and the build plan in [docs/project/plan.md](docs/project/plan.md). A change that goes against a decision should say so and why; if it's accepted, the decision is updated in the same pull request.

## How changes land

- **Trunk-based.** `main` is always releasable. Branch from it, keep the branch short-lived, and rebase on `main` rather than merging it in.
- **One pull request per change.** A fix, a feature or a refactor, not several at once. Unrelated cleanups go in their own pull request.
- **Squash merges.** Each pull request becomes one commit on `main`, so the pull request title is the commit title and the description is its body: say what changed and why.
- **Conventional Commit titles**, for example `fix(import): keep rows with a null URI`, `feat(api): add genre timelines`, `docs: explain TRUSTED_PROXIES`. Types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `ci`, `chore`. Mark breaking changes with `!` (`feat(api)!: …`) and explain them in the description; during 0.x they're allowed but must be listed in the next release notes.
- **CI must pass** (Go vet and tests, the performance budgets, web check, lint and build, the image build, DCO sign-off) before a pull request is merged. Jobs whose inputs did not change are skipped: a site, docs or `motion/` change runs only the DCO check (and the site preview), Go changes skip the web job, and web changes skip the Go and performance jobs.

## Sign your commits off (DCO)

spotussi is licensed under [AGPL-3.0-only](LICENSE), and contributions come in under the same licence. There's no CLA. Instead, every commit carries a `Signed-off-by` line certifying the [Developer Certificate of Origin](https://developercertificate.org/): that you wrote the change, or otherwise have the right to submit it under the project's licence.

```sh
git commit -s -m "fix(capture): retry after a 502 from the player endpoint"
```

Use your real name and an email you can be reached at. To sign off commits you already made: `git rebase --signoff main`.

## Development setup

Toolchains are pinned in `mise.toml` (Go, Node); run `mise install` once. Make targets call tools through `mise exec`, so mise doesn't need to be activated. Browser-facing dev servers run through [Portless](https://portless.sh), which gives them named HTTPS `.localhost` URLs (prefixed with the worktree name inside a git worktree).

```sh
make dev     # mock API + Vite dev server (HMR): https://spotussi.localhost
make mock    # production build, binary in mock mode: https://spotussi-mock.localhost
```

Without Portless: `go run ./cmd/spotussi serve --mock` (API on :8080), then `cd web && SPOTUSSI_API_URL=http://127.0.0.1:8080 pnpm dev`. Other targets: `make build` (web build + static binary at `bin/spotussi`), `make gen-api`, `make bench`, `make docker`.

### Mock mode

`spotussi serve --mock` generates a deterministic synthetic listening history in memory and serves every endpoint from it, with no Spotify account, no login and no state, so almost all UI and API work can be done against it. `--mock-size full|small|tiny` (15 years ~500k plays, ~50k, ~5k), `--mock-seed N`, `--mock-tz ZONE`. Set `AUTH_MODE=password` to work on the login screens. The live backend is tested against the mock for identical responses.

### Layout

| Path | What |
|---|---|
| `cmd/spotussi` | The binary and its subcommands |
| `internal/` | `config`, `db` (SQLite, migrations, backup/restore), `api` (HTTP, auth enforcement, ETags, SPA), `auth`, `spotify` (OAuth, Web API, rate limiter), `capture` (live capture), `importer`, `hydrate` and `metadata` (catalog lookups and fallbacks), `derive` (merge, play rule, snapshot), `live` (live backend), `mcp`, `query`, `refdata` (synthetic dataset), `mock` (mock API) |
| `api/openapi.yaml` | The `/api/v1` contract (OpenAPI 3.1) |
| `web/` | SvelteKit SPA (Svelte 5, Tailwind v4, shadcn-svelte, LayerChart), embedded via `go:embed` |
| `site/`, `motion/` | The marketing page and its Remotion hero video |
| `docs/` | User documentation; `docs/project/` holds decisions, plan and release notes |

## Before you open a pull request

```sh
make lint    # gofmt, go vet, prettier, eslint, generated client up to date
make test    # Go tests (incl. OpenAPI contract tests) + svelte-check
```

Also:

- **API changes** start in `api/openapi.yaml`. Run `make gen-api` and commit the regenerated `web/src/lib/api/schema.d.ts`; the contract tests check both the mock and the live backend against the spec.
- **Performance-sensitive changes** (derivation, queries, the snapshot) should run `make bench`; CI fails past 2.5x the budgets in docs/project/plan.md.
- **UI changes** need light and dark, desktop and phone widths, and no new axe violations. Screenshots in the pull request help.
- **Never commit real listening data,** Spotify or API tokens, `.env` files or a `data/` directory. Use the mock or the synthetic reference dataset (`internal/refdata`) in tests, fixtures and screenshots.

## Database migrations

Migrations live in `internal/db/migrations` and run in order on start. **A migration that has been in a release never changes:** existing instances have already applied it. To change the schema, add the next numbered migration. Migrations that only exist on `main` since the last release may still be edited.

## Reporting bugs and security issues

Open an issue with the version (`spotussi version`), how you run it, and what you expected. Security problems go through private reporting instead; see [SECURITY.md](SECURITY.md).

## Releases

Maintainers tag `v<major>.<minor>.<patch>[-pre]` (for example `v0.1.0-beta.2`) on `main` after adding `docs/project/release-notes/<tag>.md`. The tag runs `.github/workflows/release.yml`: lint and tests, the multi-arch image to GHCR (`:beta` follows the newest beta, `:latest` only final releases), static binaries and `SHA256SUMS` (`scripts/release-binaries.sh`, runnable locally), build provenance attestations (public repository only), and a GitHub Release from the notes file, marked as a pre-release when the tag has a suffix.
