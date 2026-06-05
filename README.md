# Loonstagram

Loonstagram is a small Go service that rewrites public Instagram post, reel, and TV URLs into Discord-friendly share URLs.

Human visitors are redirected to Instagram. Crawlers receive escaped Open Graph and Twitter Card HTML, with metadata cached in SQLite.

## Local Docker Test

```sh
cd deploy
cp .env.local.example .env.local
docker compose -f docker-compose.local.yml --env-file .env.local up --build
```

The local compose file runs Nginx without TLS at `http://localhost:8080`.

The default `docker-compose.yml` is also local-friendly, so this works too:

```sh
docker compose up --build
```

## Production Deploy

Production is configured for `https://loonstagram.com`. `www.loonstagram.com` is accepted and redirected to the apex domain.

1. Point the DNS A record for `loonstagram.com` to the VM.
   If you want `www.loonstagram.com`, point its A record to the VM too, or add a CNAME from `www` to `loonstagram.com`.
2. Open ports 80 and 443 on the VM firewall.
3. Generate the certificate on the VM:

```sh
sudo certbot certonly --standalone -d loonstagram.com -d www.loonstagram.com
```

If another service is already listening on port 80, stop it while certbot runs. The production Compose file mounts `/etc/letsencrypt` read-only into Nginx.

4. Start the production stack:

```sh
cd deploy
cp .env.prod.example .env.prod
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --build
```

The production Dockerfile only builds the release binary. Run tests before deploying:

```sh
go test ./...
```

If `go.sum` is missing after dependency changes, regenerate it before deploying:

```sh
go mod tidy
```

5. Check it:

```sh
curl https://loonstagram.com/healthz
curl -A "Discordbot/2.0" "https://loonstagram.com/reel/ABC123xyz?preview=1"
```

For cert renewal with standalone certbot, stop the Nginx container before renewal and start it again after renewal:

```sh
docker compose -f deploy/docker-compose.prod.yml stop nginx
sudo certbot renew
docker compose -f deploy/docker-compose.prod.yml up -d nginx
```

## Local App Settings

Required:

- `PUBLIC_BASE_URL`, for example `https://loonstagram.com`
- `DATABASE_PATH`, for example `/data/Loonstagram.sqlite`

Optional defaults:

- `LISTEN_ADDR=:3000`
- `CACHE_SUCCESS_TTL=6h`
- `CACHE_NEGATIVE_TTL=15m`
- `CACHE_BLOCKED_TTL=5m`
- `HTTP_CLIENT_TIMEOUT=8s`
- `MEDIA_PROXY_MODE=redirect`
- `ENABLE_INSTAGRAM_GQL_FALLBACK=false` (reserved for later fallback support)
- `ADMIN_TOKEN=` (when set, unlocks automation settings in the web UI)
- `AUTOMATION_POLL_INTERVAL=2m`
- `INSTAGRAM_WEB_APP_ID=936619743392459`
- `INSTAGRAM_SESSION_ID=` (optional self-hosted fallback for Instagram profile polling)
- `DISCORD_CLIENT_ID=` (optional, enables Discord channel connection through OAuth)
- `DISCORD_CLIENT_SECRET=` (optional, required with `DISCORD_CLIENT_ID`)
- `DISCORD_REDIRECT_URL=` (optional, defaults to `{PUBLIC_BASE_URL}/oauth/discord/callback`)
- `LOG_LEVEL=info`

## Supported Input URLs

- `https://www.instagram.com/p/{shortcode}/`
- `https://instagram.com/p/{shortcode}/`
- `https://www.instagram.com/reel/{shortcode}/`
- `https://instagram.com/reel/{shortcode}/`
- `https://www.instagram.com/reels/{shortcode}/`
- `https://www.instagram.com/tv/{shortcode}/`
- `https://www.instagram.com/{username}/p/{shortcode}/`
- `https://www.instagram.com/{username}/reel/{shortcode}/`

## Manual Checks

```sh
curl http://localhost:8080/healthz
curl -X POST http://localhost:8080/api/convert \
  -H "Content-Type: application/json" \
  -d '{"url":"https://www.instagram.com/reel/ABC123xyz/"}'
curl -A "Discordbot/2.0" "http://localhost:8080/reel/ABC123xyz?preview=1"
```

## Instagram to Discord Automation

Set `ADMIN_TOKEN` before exposing automation settings. The public converter stays open, but automation API writes require the admin token through the web UI.
Stored Discord webhook URLs are encrypted with a key derived from `ADMIN_TOKEN`, so keep that value stable after connecting Discord.

Discord can be connected in two ways:

- Paste a Discord webhook URL in the admin panel.
- Configure `DISCORD_CLIENT_ID` and `DISCORD_CLIENT_SECRET`, then use the Connect Discord button. The Discord app redirect URL must match `DISCORD_REDIRECT_URL` or `{PUBLIC_BASE_URL}/oauth/discord/callback`.

Instagram polling watches a public username through Instagram's web profile endpoint. The first successful poll fetches and caches the current recent posts for the gallery without posting them to Discord. Later unseen shortcodes are cached too; they are delivered only when Discord is already connected. If Instagram blocks the no-login profile endpoint, self-hosted deployments can optionally provide `INSTAGRAM_SESSION_ID`; treat that as a sensitive credential.

## Debug URLs

Open:

```text
http://localhost:8080/debug/p/ABC123xyz
http://localhost:8080/debug?url=https%3A%2F%2Fwww.instagram.com%2Fp%2FABC123xyz%2F
```

The debug page performs fresh Instagram fetches, shows cache state, raw upstream bodies, extracted JSON blocks, parsed post data, media previews, and fetch or parse errors. Response headers that can carry secrets, such as `Set-Cookie`, are redacted.

Private or login-only Instagram content is not supported. Metadata scraping is best effort and falls back to a minimal preview when Instagram cannot be fetched.
