# Security policy

## Supported versions

spotussi is in beta. Only the latest release gets security fixes: today that is the newest `v0.1.0-beta.N` tag (image `ghcr.io/jasoncronje/spotussi:beta`). Fixes ship as a new beta, not as patches to older ones, so update to the latest release before reporting.

| Version | Supported |
|---|---|
| Latest `0.1.0-beta.N` | Yes |
| Older betas, unreleased `main` | No |

## Reporting a vulnerability

Please report vulnerabilities privately, not in a public issue, pull request or discussion.

1. Open the repository's **Security** tab and choose **Report a vulnerability** ([GitHub private vulnerability reporting](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)). This opens a draft security advisory that only you and the maintainer can see.
2. Include the version (`spotussi version`), how the instance is run (Docker, binary, behind which proxy, `AUTH_MODE`), the steps to reproduce, and what an attacker gains.
3. Don't include real listening history, Spotify tokens, API tokens or the instance key. Synthetic data from mock mode (`spotussi serve --mock`) is enough to show most problems.

You should get an answer within a week. Once a fix is released, the advisory is published with credit to you unless you'd rather stay anonymous.

## Scope

In scope: the `spotussi` binary and image (HTTP API, authentication and sessions, API tokens, the MCP server, the importer, backup and restore, the stored Spotify connection), the release workflow and its artifacts.

Out of scope: problems that need an attacker who already controls the host, the `/data` volume or a trusted reverse proxy; instances run with `AUTH_MODE=none` on a reachable network (the UI warns about it); missing hardening headers without a concrete attack; vulnerabilities in Spotify, Deezer, MusicBrainz or your proxy, which belong with those projects.

## Verifying releases

Release archives come with `SHA256SUMS`. Images and archives carry GitHub build provenance attestations, which you can check with the GitHub CLI:

```sh
gh attestation verify oci://ghcr.io/jasoncronje/spotussi:0.1.0-beta.2 --repo jasoncronje/spotussi
gh attestation verify spotussi_0.1.0-beta.2_linux_amd64.tar.gz --repo jasoncronje/spotussi
```
