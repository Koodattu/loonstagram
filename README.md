# InstaFix

InstaFix is a small Go service that rewrites public Instagram post, reel, and TV URLs into Discord-friendly share URLs.

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

Production is configured for `https://loonstagram.com`.

1. Point the DNS A record for `loonstagram.com` to the VM.
2. Open ports 80 and 443 on the VM firewall.
3. Generate the certificate on the VM:

```sh
sudo certbot certonly --standalone -d loonstagram.com
```

If another service is already listening on port 80, stop it while certbot runs. The production Compose file mounts `/etc/letsencrypt` read-only into Nginx.

4. Start the production stack:

```sh
cd deploy
cp .env.prod.example .env.prod
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --build
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
- `DATABASE_PATH`, for example `/data/instafix.sqlite`

Optional defaults:

- `LISTEN_ADDR=:3000`
- `CACHE_SUCCESS_TTL=6h`
- `CACHE_NEGATIVE_TTL=15m`
- `CACHE_BLOCKED_TTL=5m`
- `HTTP_CLIENT_TIMEOUT=8s`
- `MEDIA_PROXY_MODE=redirect`
- `ENABLE_INSTAGRAM_GQL_FALLBACK=false` (reserved for later fallback support)
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

Private or login-only Instagram content is not supported. Metadata scraping is best effort and falls back to a minimal preview when Instagram cannot be fetched.
