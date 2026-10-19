# Install and update

spotussi ships as a multi-arch image (`linux/amd64`, `linux/arm64`; distroless, static, non-root) and as static binaries. Everything it stores lives in one data directory (`/data` in the image): `spotussi.db` (SQLite) and `key`, the instance key that encrypts the Spotify connection. Back both up ([backup.md](backup.md)).

You also need a Spotify account with Premium: Spotify only lets Premium accounts create the developer app spotussi connects through ([spotify.md](spotify.md)).

## Docker Compose

```yaml
services:
  spotussi:
    image: ghcr.io/jasoncronje/spotussi:0.1.0-beta.2
    container_name: spotussi
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - spotussi-data:/data
    environment:
      TZ: Europe/London # default display time zone until you pick one
volumes:
  spotussi-data:
```

```sh
docker compose up -d
docker compose logs spotussi | grep -A3 "setup token"
```

Open `http://<server>:8080`, enter the setup token and choose an owner password. The wizard then connects Spotify. `:beta` follows the newest beta if you'd rather not pin a version.

## HTTPS behind Traefik

Over HTTPS, Spotify redirects straight back to your instance after you approve it. spotussi ignores `X-Forwarded-*` headers unless the proxy is listed in `TRUSTED_PROXIES`; without it, it offers the plain-HTTP redirect URI and logs a warning naming the address to add.

```yaml
services:
  spotussi:
    image: ghcr.io/jasoncronje/spotussi:0.1.0-beta.2
    container_name: spotussi
    restart: unless-stopped
    volumes:
      - spotussi-data:/data
    networks: [traefik]
    environment:
      TZ: Europe/London
      # Traefik's address as the container sees it (here Docker's private range).
      TRUSTED_PROXIES: 172.16.0.0/12
      # Optional: pin the external URL instead of deriving it per request.
      # BASE_URL: https://stats.example.com
    labels:
      - traefik.enable=true
      - traefik.docker.network=traefik
      - traefik.http.routers.spotussi.rule=Host(`stats.example.com`)
      - traefik.http.routers.spotussi.entrypoints=websecure
      - traefik.http.routers.spotussi.tls.certresolver=letsencrypt
      - traefik.http.services.spotussi.loadbalancer.server.port=8080
volumes:
  spotussi-data:
networks:
  traefik:
    external: true
```

The redirect URI to register with Spotify is then `https://stats.example.com/oauth/spotify/callback`; the wizard shows it.

## Single sign-on (forward auth)

Set `AUTH_MODE=proxy` to sign in through your forward-auth provider instead of an owner password. spotussi reads the user from `Remote-User` (`AUTH_PROXY_HEADER`), and only from `TRUSTED_PROXIES`.

API and MCP clients use API tokens and can't pass the SSO login page, so give their requests a second router without forward auth. That router must strip `Remote-User`, or anyone could send the header themselves:

```yaml
    environment:
      AUTH_MODE: proxy
      TRUSTED_PROXIES: 172.16.0.0/12
    labels:
      # ... the router above, plus:
      - traefik.http.routers.spotussi.middlewares=my-forward-auth@file
      # Token clients: /mcp, and any request with a spotussi API token.
      - traefik.http.routers.spotussi-token.rule=Host(`stats.example.com`) && (PathPrefix(`/mcp`) || HeaderRegexp(`Authorization`, `^Bearer spt_`))
      - traefik.http.routers.spotussi-token.priority=1000
      - traefik.http.routers.spotussi-token.entrypoints=websecure
      - traefik.http.routers.spotussi-token.tls.certresolver=letsencrypt
      - traefik.http.routers.spotussi-token.service=spotussi
      - traefik.http.routers.spotussi-token.middlewares=spotussi-strip-user
      - traefik.http.middlewares.spotussi-strip-user.headers.customrequestheaders.Remote-User=
```

`AUTH_MODE=none` skips login entirely. Use it only on a private network or VPN; the UI shows a warning.

## Standalone binaries

Each [release](https://github.com/jasoncronje/spotussi/releases) has static binaries for Linux and macOS (amd64, arm64) with the UI built in, plus `SHA256SUMS`:

```sh
sha256sum -c SHA256SUMS --ignore-missing   # macOS: shasum -a 256 -c SHA256SUMS --ignore-missing
tar -xzf spotussi_0.1.0-beta.2_linux_amd64.tar.gz
DATA_DIR=/var/lib/spotussi ./spotussi_0.1.0-beta.2_linux_amd64/spotussi serve
```

The macOS binaries aren't notarized; allow them with `xattr -d com.apple.quarantine spotussi`. A minimal systemd unit:

```ini
[Unit]
Description=spotussi
After=network-online.target

[Service]
ExecStart=/usr/local/bin/spotussi serve
Environment=DATA_DIR=/var/lib/spotussi PORT=8080
DynamicUser=yes
StateDirectory=spotussi
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

## Updating

Read the [release notes](https://github.com/jasoncronje/spotussi/releases) first. During 0.x an update can change the API or trigger a full rebuild of derived data (it runs automatically on start). Migrations run on start and only go forward, so take a backup before updating.

```sh
docker compose pull && docker compose up -d
```
