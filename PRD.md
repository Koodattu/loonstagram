# Loonstagram Rebuild PRD

## 1. Summary

Build a lightweight Go service that turns Instagram post, reel, and video URLs into Discord-friendly share URLs.

The product should work like the current Loonstagram idea:

1. A user pastes an Instagram URL into a small web UI.
2. The service returns a rewritten URL on our domain.
3. When that rewritten URL is posted to Discord, Discord sees a useful rich embed.
4. When a human clicks the rewritten URL, they are sent to the original Instagram page.

The implementation should be boring, small, deployable with Docker Compose on a VPS, and avoid unnecessary moving parts. Go is the chosen backend language. Nginx should run as a container in front of the Go app. SQLite should be used only as a local cache, not as a heavy application database.

## 2. Goals

- Create Discord-friendly Instagram link previews for public Instagram posts and reels.
- Preserve the original Instagram click-through behavior for human users.
- Provide a tiny web UI for non-technical users to paste an Instagram URL and copy the fixed URL.
- Keep deployment simple: Docker Compose, Go app container, Nginx container, SQLite volume.
- Keep runtime lightweight and cheap enough for a small VPS.
- Reuse the useful tricks from the old project:
  - Bot/user-agent detection.
  - Serve crawler metadata to Discord.
  - Redirect humans to Instagram.
  - Use Open Graph and Twitter Card metadata.
  - Use media proxy/redirect endpoints instead of exposing raw scraped URLs in the public page.
  - Cache scraped Instagram metadata.
- Make failure behavior predictable. If the service cannot fetch metadata, the generated URL should still redirect humans to Instagram and show a minimal fallback preview to crawlers.

## 3. Non-Goals

- No Discord bot for MVP.
- No user accounts.
- No admin dashboard for MVP.
- No queue system for MVP.
- No Kubernetes setup.
- No multi-node deployment for MVP.
- No full Instagram client or feed reader.
- No private Instagram content support.
- No permanent media archival.
- No attempt to bypass login-only or private content.
- No browser automation in MVP unless scraping without it proves impossible.

## 4. Users

### Casual sharer

Wants to paste an Instagram link and get a better Discord link without understanding URL rewriting.

Needs:

- A web page with one input.
- A copy button.
- Clear error text if the URL is unsupported.

### Power user

Already knows the URL pattern and edits links manually.

Needs:

- Stable canonical paths like `/p/{shortcode}` and `/reel/{shortcode}`.
- Predictable redirect behavior when clicked.

### Discord crawler

Fetches the rewritten URL to build a link preview.

Needs:

- HTTP 200 HTML response.
- Open Graph and Twitter Card metadata in the initial HTML.
- Absolute HTTPS media URLs.
- Media endpoints that return or redirect to valid image/video content.

## 5. Product Behavior

### 5.1 User-facing web UI

Route: `GET /`

The page should contain:

- One input field for an Instagram URL.
- One submit button.
- One result field containing the generated fixed URL.
- One copy button.
- Minimal validation feedback.

The UI should not need client-side framework code. Plain HTML, CSS, and a small amount of vanilla JavaScript is enough.

Example flow:

1. User opens `https://fix.example.com`.
2. User pastes `https://www.instagram.com/reel/ABC123xyz/`.
3. UI returns `https://fix.example.com/reel/ABC123xyz`.
4. User copies that URL and posts it to Discord.

### 5.2 Direct URL rewriting

Supported input URLs:

- `https://www.instagram.com/p/{shortcode}/`
- `https://instagram.com/p/{shortcode}/`
- `https://www.instagram.com/reel/{shortcode}/`
- `https://instagram.com/reel/{shortcode}/`
- `https://www.instagram.com/reels/{shortcode}/`
- `https://www.instagram.com/tv/{shortcode}/`
- `https://www.instagram.com/{username}/p/{shortcode}/`
- `https://www.instagram.com/{username}/reel/{shortcode}/`

Optional later support:

- `https://www.instagram.com/share/reel/{shareID}/`
- `https://www.instagram.com/share/p/{shareID}/`
- Story URLs, if public metadata can be fetched reliably.

Canonical output URLs:

- `https://fix.example.com/p/{shortcode}`
- `https://fix.example.com/reel/{shortcode}`
- `https://fix.example.com/tv/{shortcode}`

### 5.3 Human click behavior

When a normal browser visits a canonical URL, the app should redirect to Instagram:

- `/p/{shortcode}` -> `https://www.instagram.com/p/{shortcode}/`
- `/reel/{shortcode}` -> `https://www.instagram.com/reel/{shortcode}/`
- `/tv/{shortcode}` -> `https://www.instagram.com/tv/{shortcode}/`

Response:

- `302 Found`
- `Location: https://www.instagram.com/...`

Exception:

- `?preview=1` should force the crawler HTML response for manual debugging.

### 5.4 Crawler behavior

When Discord or another known crawler visits a canonical URL, the app should return an HTML page with metadata instead of redirecting.

Response:

- `200 OK`
- `Content-Type: text/html; charset=utf-8`
- No JavaScript required for metadata.
- Metadata must be in the first response body.

Required metadata:

```html
<meta property="og:site_name" content="Loonstagram">
<meta property="og:type" content="article">
<meta property="og:title" content="@username on Instagram">
<meta property="og:description" content="Caption preview">
<meta property="og:url" content="https://www.instagram.com/reel/ABC123xyz/">
<meta property="og:image" content="https://fix.example.com/media/ABC123xyz/1/image">

<meta name="twitter:card" content="summary_large_image">
<meta name="twitter:title" content="@username on Instagram">
<meta name="twitter:description" content="Caption preview">
<meta name="twitter:image" content="https://fix.example.com/media/ABC123xyz/1/image">
```

Video metadata, when available and verified:

```html
<meta property="og:video" content="https://fix.example.com/media/ABC123xyz/1/video">
<meta property="og:video:secure_url" content="https://fix.example.com/media/ABC123xyz/1/video">
<meta property="og:video:type" content="video/mp4">
<meta name="twitter:card" content="player">
<meta name="twitter:player:stream" content="https://fix.example.com/media/ABC123xyz/1/video">
<meta name="twitter:player:stream:content_type" content="video/mp4">
```

The HTML body should include a plain link to the original Instagram page:

```html
<a href="https://www.instagram.com/reel/ABC123xyz/">Open on Instagram</a>
```

Do not rely only on `<meta http-equiv="refresh">`. It is acceptable as a fallback, but HTTP redirect for humans and crawler-specific HTML are the primary behavior.

## 6. Technical Approach

## 6.1 High-level architecture

```text
Internet
  |
  v
Nginx container
  |
  v
Go app container
  |
  v
SQLite cache on Docker volume
```

Nginx responsibilities:

- Public HTTP/HTTPS entrypoint.
- Reverse proxy to Go app.
- Request size limits.
- Basic rate limiting.
- Optional static asset caching.
- Optional TLS termination, depending on VPS setup.

Go app responsibilities:

- Parse and normalize Instagram URLs.
- Detect crawler vs human user-agent.
- Fetch Instagram metadata when needed.
- Cache post metadata in SQLite.
- Render metadata HTML.
- Serve the simple converter UI.
- Serve media redirect/proxy endpoints.
- Expose healthcheck endpoint.

SQLite responsibilities:

- TTL cache for scraped post metadata.
- Negative cache for repeated failed fetches.
- No user data.
- No auth data.

## 6.2 Suggested repository structure

```text
cmd/Loonstagram/
  main.go

internal/app/
  config.go
  server.go

internal/httpx/
  handlers.go
  middleware.go
  crawler.go
  templates.go

internal/instagram/
  normalize.go
  scrape.go
  parse_embed.go
  parse_graphql.go
  types.go

internal/cache/
  sqlite.go
  schema.sql

web/
  templates/
    home.html
    embed.html
  static/
    app.css
    app.js

deploy/
  docker-compose.yml
  nginx.conf
  Dockerfile
  .env.example
```

This can also be implemented in the current flat repo at first, but the above structure is cleaner for a rebuild.

## 6.3 Go dependencies

Keep dependencies small.

Preferred:

- Standard library `net/http` and Go 1.22+ `http.ServeMux` path patterns.
- `html/template` for HTML rendering.
- `encoding/json` for cache serialization.
- `database/sql`.
- `modernc.org/sqlite` for pure-Go SQLite.
- `github.com/tidwall/gjson` if JSON extraction from Instagram script blobs is needed.
- `golang.org/x/net/html` if DOM parsing is needed.

Avoid for MVP:

- Full frontend framework.
- Heavy ORM.
- Browser automation.
- Redis.
- Message queue.
- Separate database server.

## 7. Instagram Metadata Fetching

Instagram is the fragile part. The app should isolate all Instagram-specific logic under `internal/instagram` so it can be patched without touching routing or deployment.

### 7.1 Normalization

Never fetch arbitrary user-submitted URLs directly.

Process:

1. Parse the submitted URL.
2. Verify the host is one of:
   - `instagram.com`
   - `www.instagram.com`
   - `m.instagram.com`
3. Extract route type and shortcode.
4. Validate shortcode using a strict allowlist:

```text
^[A-Za-z0-9_-]{5,128}$
```

5. Construct the Instagram URL server-side from the normalized type and shortcode.

This prevents SSRF and open proxy behavior.

### 7.2 Primary metadata source: Instagram embed page

First attempt:

```text
GET https://www.instagram.com/{type}/{shortcode}/embed/captioned/
```

This matches the old project's main trick.

The scraper should try to extract:

- Username.
- Caption.
- Thumbnail/image URL.
- Video URL, if present.
- Media type.
- Multiple media items, if present.

Extraction strategies:

1. Search script content for structured data containing `shortcode_media` or similar keys.
2. Parse JSON from that script safely.
3. Fallback to HTML selectors for embedded image/video elements.
4. Fallback to generic `og:image`, `og:title`, and `og:description` from the Instagram page.

The old project currently searches the embed HTML and script tags for `shortcode_media`, then falls back to DOM selectors and GraphQL. The rebuild should keep that idea but isolate it behind a clean `FetchPost(ctx, ref)` function.

### 7.3 Secondary metadata source: GraphQL fallback

If the embed page fails or the video is blocked, the app may try Instagram's web GraphQL endpoint as a fallback.

This should be feature-flagged:

```text
ENABLE_INSTAGRAM_GQL_FALLBACK=false
```

Reason:

- The GraphQL request shape is undocumented.
- Hard-coded document IDs and headers can break without warning.
- It is useful as a best-effort fallback, not as the core contract.

If enabled, keep all GraphQL request parameters in one file and log a clear error when it fails.

### 7.4 Future optional source: official oEmbed

If Meta oEmbed access is available later, add it as a first-class provider:

```text
INSTAGRAM_OEMBED_ACCESS_TOKEN=
```

Provider order could become:

1. SQLite cache.
2. Official oEmbed, if configured.
3. Instagram embed page scrape.
4. GraphQL fallback, if enabled.

Do not require this for MVP, because app review and token management add friction.

## 8. Data Model

SQLite should be treated as disposable cache.

### 8.1 `posts` table

```sql
CREATE TABLE posts (
  shortcode TEXT NOT NULL,
  media_type TEXT NOT NULL,
  original_url TEXT NOT NULL,
  username TEXT,
  caption TEXT,
  media_json TEXT NOT NULL,
  status TEXT NOT NULL,
  error TEXT,
  fetched_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  PRIMARY KEY (shortcode, media_type)
);

CREATE INDEX idx_posts_expires_at ON posts (expires_at);
```

`media_json` example:

```json
[
  {
    "kind": "image",
    "url": "https://scontent.cdninstagram.com/...",
    "width": 1080,
    "height": 1350,
    "contentType": "image/jpeg"
  },
  {
    "kind": "video",
    "url": "https://scontent.cdninstagram.com/...",
    "posterUrl": "https://scontent.cdninstagram.com/...",
    "width": 720,
    "height": 1280,
    "contentType": "video/mp4"
  }
]
```

### 8.2 Cache TTLs

Recommended defaults:

- Successful metadata fetch: 6 hours.
- Successful media URL refresh: same as metadata.
- Not found/private/login required: 15 minutes.
- Scrape blocked/rate limited: 5 minutes.

Reason:

- Instagram CDN URLs can expire.
- Discord itself also caches embeds, so overly long local cache is not very useful.
- Short negative cache prevents hammering Instagram on repeated bad links.

### 8.3 Cache cleanup

Run cleanup in-app every 10 minutes:

```sql
DELETE FROM posts WHERE expires_at < unixepoch();
```

Keep it simple. No external cron required.

## 9. HTTP API

### 9.1 `GET /`

Renders the converter UI.

### 9.2 `POST /api/convert`

Request:

```json
{
  "url": "https://www.instagram.com/reel/ABC123xyz/"
}
```

Response success:

```json
{
  "ok": true,
  "url": "https://fix.example.com/reel/ABC123xyz",
  "type": "reel",
  "shortcode": "ABC123xyz"
}
```

Response error:

```json
{
  "ok": false,
  "error": "Unsupported Instagram URL"
}
```

Do not fetch Instagram metadata in this endpoint for MVP. Conversion should be instant and deterministic.

### 9.3 `GET /p/{shortcode}`

Canonical post URL.

Human:

- Redirects to Instagram.

Crawler:

- Returns metadata HTML.

### 9.4 `GET /reel/{shortcode}`

Canonical reel URL.

Human:

- Redirects to Instagram.

Crawler:

- Returns metadata HTML.

### 9.5 `GET /tv/{shortcode}`

Canonical legacy IGTV URL.

Human:

- Redirects to Instagram.

Crawler:

- Returns metadata HTML.

### 9.6 `GET /media/{type}/{shortcode}/{index}/image`

Returns or redirects to an image for the media item.

Behavior:

- Look up cached metadata.
- If missing or expired, refresh metadata.
- If an image URL exists, either:
  - `302` redirect to the Instagram CDN URL, or
  - stream the image through the app if `MEDIA_PROXY_MODE=stream`.
- Set `Content-Type` correctly when streaming.
- Set conservative cache headers.

Default for MVP:

```text
MEDIA_PROXY_MODE=redirect
```

### 9.7 `GET /media/{type}/{shortcode}/{index}/video`

Returns, redirects, or streams video.

Behavior:

- Look up cached metadata.
- If missing or expired, refresh metadata.
- If a video URL exists:
  - Prefer streaming through the app with Range support when Discord needs it.
  - Otherwise redirect to the CDN URL.
- If video fails, fallback to image preview in the metadata HTML.

This endpoint is allowed to be less perfect in MVP. Image embeds are the baseline success case.

### 9.8 `GET /oembed`

Optional, not required for MVP.

If included, implement with `json.Encoder`, not string concatenation.

Request:

```text
/oembed?url=https%3A%2F%2Ffix.example.com%2Freel%2FABC123xyz
```

Response:

```json
{
  "version": "1.0",
  "type": "link",
  "provider_name": "Loonstagram",
  "provider_url": "https://fix.example.com",
  "title": "@username on Instagram",
  "author_name": "@username",
  "author_url": "https://www.instagram.com/username/"
}
```

### 9.9 `GET /healthz`

Returns:

```json
{
  "ok": true
}
```

Used by Docker healthcheck.

## 10. Crawler Detection

The existing project checks user-agent strings for known bots. Keep that approach, but make it explicit and testable.

Known crawler fragments:

- `discordbot`
- `discord`
- `facebookexternalhit`
- `twitterbot`
- `slackbot`
- `telegrambot`
- `whatsapp`
- `linkedinbot`
- `embedly`
- `preview`
- `bot`
- `crawl`
- `spider`

Rules:

- Match case-insensitively.
- `?preview=1` always acts like a crawler.
- `?redirect=1` always acts like a human redirect.
- Unknown user-agents should be treated as human by default.

Important:

- Do not put expensive scrape work in the human path.
- Humans should redirect before metadata fetch.
- Crawlers should fetch metadata and render HTML.

## 11. Rendering Rules

Use `html/template`.

Escape all user-controlled values:

- Caption.
- Username.
- Original URL.
- Media URLs.
- Error messages.

Caption handling:

- Trim whitespace.
- Collapse excessive blank lines.
- Limit to 280 characters for embed description.
- Do not include untrusted HTML.

Title format:

```text
@username on Instagram
```

Fallback title:

```text
Instagram post
```

Fallback description:

```text
Open this Instagram post.
```

Fallback image:

- Optional static branded image, served from `/static/fallback.png`.
- If not available, omit `og:image` rather than returning a broken URL.

## 12. Media Handling

### 12.1 Image strategy

MVP:

- Redirect image requests to Instagram CDN URLs.
- Use our `/media/.../image` URL in metadata.

Why:

- Keeps server bandwidth low.
- Matches the old app's lightweight behavior.

Possible issue:

- Discord may cache or follow the CDN redirect differently over time.

Fallback:

- Add `MEDIA_PROXY_MODE=stream` to stream image content through the app.

### 12.2 Video strategy

MVP baseline:

- Use image/poster preview for reels and videos.

Enhanced mode:

- Stream MP4 through `/media/.../video`.
- Support `Range` requests.
- Set:
  - `Content-Type: video/mp4`
  - `Accept-Ranges: bytes`
  - `Content-Length`
  - `206 Partial Content` for range responses.

Do not buffer whole videos in memory.

If video proxying is unreliable, prefer a stable image embed over broken video metadata.

### 12.3 Gallery strategy

MVP:

- Use the first media item as the embed image.

Later:

- Add `/gallery/{type}/{shortcode}` or a generated grid image like the old app.

Reason to skip in MVP:

- Grid generation adds image processing, local file cache, and more failure modes.
- First-image preview is good enough for the core use case.

## 13. Configuration

Environment variables:

```text
PUBLIC_BASE_URL=https://fix.example.com
LISTEN_ADDR=:3000
DATABASE_PATH=/data/Loonstagram.sqlite
CACHE_SUCCESS_TTL=6h
CACHE_NEGATIVE_TTL=15m
CACHE_BLOCKED_TTL=5m
HTTP_CLIENT_TIMEOUT=8s
MEDIA_PROXY_MODE=redirect
ENABLE_INSTAGRAM_GQL_FALLBACK=false
INSTAGRAM_OEMBED_ACCESS_TOKEN=
LOG_LEVEL=info
```

Required:

- `PUBLIC_BASE_URL`
- `DATABASE_PATH`

Optional:

- Everything else can have defaults.

Startup should fail fast if:

- `PUBLIC_BASE_URL` is missing.
- `PUBLIC_BASE_URL` is not absolute HTTP/HTTPS.
- SQLite cannot be opened.

## 14. Docker Deployment

### 14.1 Services

Use Docker Compose with two services:

```yaml
services:
  app:
    build:
      context: ..
      dockerfile: deploy/Dockerfile
    environment:
      PUBLIC_BASE_URL: https://fix.example.com
      DATABASE_PATH: /data/Loonstagram.sqlite
      LISTEN_ADDR: :3000
    volumes:
      - app-data:/data
    expose:
      - "3000"
    restart: unless-stopped

  nginx:
    image: nginx:1.27-alpine
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
      - ./certs:/etc/nginx/certs:ro
    ports:
      - "80:80"
      - "443:443"
    depends_on:
      - app
    restart: unless-stopped

volumes:
  app-data:
```

### 14.2 Go app Dockerfile

Use multi-stage build:

```dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/Loonstagram ./cmd/Loonstagram

FROM alpine:3.21
RUN adduser -D -H -u 10001 appuser
USER appuser
COPY --from=build /out/Loonstagram /usr/local/bin/Loonstagram
EXPOSE 3000
ENTRYPOINT ["/usr/local/bin/Loonstagram"]
```

If using `modernc.org/sqlite`, `CGO_ENABLED=0` should be possible. If using `mattn/go-sqlite3`, CGO must be enabled and the runtime image gets more complicated. Prefer pure-Go SQLite for this project.

### 14.3 Nginx behavior

Nginx should:

- Proxy all requests to `app:3000`.
- Preserve `Host`, `X-Forwarded-Proto`, and `X-Forwarded-For`.
- Limit request body size to something tiny, such as `64k`.
- Apply basic rate limits to scrape-triggering endpoints.
- Use gzip for HTML/CSS/JS.

Example limits:

```nginx
limit_req_zone $binary_remote_addr zone=general:10m rate=5r/s;
limit_req_zone $binary_remote_addr zone=expensive:10m rate=30r/m;
```

Apply the expensive limit to:

- `/p/`
- `/reel/`
- `/tv/`
- `/media/`

### 14.4 TLS

Simplest production choices:

1. Put Cloudflare in front and let Nginx serve HTTP internally.
2. Mount Let's Encrypt certs into the Nginx container from the VPS.
3. Add a separate certbot/acme companion later.

Do not put certificate renewal logic inside the Go app.

## 15. Observability

Logging:

- Structured JSON logs.
- One log line per request.
- Include request ID.
- Include route, status code, duration, user-agent category, cache hit/miss.
- Do not log full query strings if they might contain tokens later.

Scrape logs:

- `shortcode`
- `media_type`
- `provider` used: `cache`, `embed_page`, `graphql`, `oembed`
- success/failure
- duration
- sanitized error reason

Metrics:

MVP can skip Prometheus.

Add `/healthz` only.

Later metrics:

- Cache hit rate.
- Scrape success rate.
- Scrape provider breakdown.
- Instagram block/error rate.
- Media proxy bytes.

## 16. Security Requirements

### 16.1 SSRF prevention

- Never proxy arbitrary URLs from user input.
- Only normalize supported Instagram URLs.
- Construct Instagram fetch URLs server-side.
- Media endpoints must look up stored media URLs from cache by shortcode and index.
- Media endpoints must not accept `?url=...`.

### 16.2 Open redirect prevention

- Human redirects must only go to constructed Instagram URLs.
- Do not accept arbitrary redirect targets.

### 16.3 HTML/JSON escaping

- Use `html/template` for HTML.
- Use `json.Encoder` for JSON.
- Do not hand-build JSON strings.

The old project has manually assembled JSON in its oEmbed path. The rebuild should avoid that pattern.

### 16.4 Rate limiting

Nginx:

- Basic IP rate limits.
- Tighter limits for metadata and media endpoints.

App:

- Singleflight request coalescing by `{type}:{shortcode}` so concurrent Discord requests do not trigger duplicate scrapes.
- HTTP client timeout.
- Max response body size when fetching Instagram HTML, such as 2 MB.

### 16.5 Secrets

- No secrets required for MVP.
- If official oEmbed token is added later, load from environment.
- Never include access tokens in generated HTML, logs, or URLs.

## 17. Performance Requirements

Targets on a small VPS:

- Human redirect path: under 30 ms app time.
- Cached crawler embed path: under 50 ms app time.
- Cold scrape path: under 2 seconds if Instagram responds normally.
- SQLite DB size: small and bounded by TTL cleanup.
- Memory: under 100 MB under normal traffic.

Concurrency:

- Use a shared `http.Client`.
- Use context timeouts.
- Use singleflight for duplicate scrape requests.
- Do not spawn unbounded goroutines.

## 18. Error Behavior

### 18.1 Invalid input in UI/API

Return:

```text
Unsupported Instagram URL
```

### 18.2 Invalid canonical path

Return:

- Human: redirect to `https://www.instagram.com/` only if path looks intentionally malformed.
- API/crawler: `404 Not Found`.

### 18.3 Metadata fetch fails

Crawler response should still be valid HTML:

- `og:title`: `Instagram post`
- `og:description`: `Open this Instagram post.`
- `og:url`: original Instagram URL
- no media tag if media is unknown

Human response:

- Redirect to Instagram as usual.

Cache:

- Store failure with short negative TTL.

### 18.4 Media endpoint fails

Return:

- `404` if media index does not exist.
- `502` if upstream media fetch fails while streaming.
- Redirect to Instagram page as a last-resort fallback only for browser-like user-agents.

## 19. Testing Plan

### 19.1 Unit tests

URL normalization:

- Accept `/p/{shortcode}`.
- Accept `/reel/{shortcode}`.
- Accept username-prefixed URLs.
- Reject non-Instagram hosts.
- Reject unsupported Instagram paths.
- Reject suspicious shortcodes.

Crawler detection:

- Discordbot is crawler.
- Regular Chrome is human.
- `?preview=1` forces crawler.
- `?redirect=1` forces human.

HTML rendering:

- Escapes caption HTML.
- Includes absolute media URLs.
- Includes original Instagram URL.

Cache:

- Cache hit returns stored metadata.
- Expired rows are ignored.
- Negative cache prevents immediate repeated fetch.

### 19.2 Integration tests

Use recorded Instagram HTML fixtures rather than live Instagram for normal CI.

Fixtures:

- Image post.
- Reel/video post.
- Multi-image carousel.
- Deleted/private/login-required response.
- Changed/missing script data fallback.

Tests:

- Scraper extracts username.
- Scraper extracts caption.
- Scraper extracts image URL.
- Scraper handles missing video URL.
- Scraper returns a typed error for not found/private.

### 19.3 Manual tests

With local Docker Compose:

1. Open web UI.
2. Convert a known public Instagram post.
3. `curl -A "Discordbot/2.0" http://localhost/p/{shortcode}?preview=1`.
4. Confirm HTML contains `og:title`, `og:url`, and `og:image`.
5. Visit the same URL in a browser and confirm redirect to Instagram.
6. Paste deployed URL into Discord and confirm a preview appears.

## 20. MVP Acceptance Criteria

MVP is complete when:

- Docker Compose starts app and Nginx.
- `/healthz` returns OK.
- `/` renders a working paste-and-copy UI.
- `/api/convert` accepts valid Instagram post/reel URLs and rejects invalid URLs.
- `/p/{shortcode}` and `/reel/{shortcode}` redirect normal browsers to Instagram.
- `/p/{shortcode}?preview=1` and `/reel/{shortcode}?preview=1` render metadata HTML.
- Discordbot user-agent receives metadata HTML without redirect.
- Metadata is cached in SQLite.
- Expired cache rows are cleaned up.
- HTML output is escaped.
- No endpoint proxies arbitrary user-supplied URLs.
- App can produce at least image-based embeds for public posts/reels.

## 21. Later Enhancements

- Better reel video playback through `/media/.../video` with Range support.
- Carousel grid generation.
- Admin-only cache purge endpoint.
- Official Meta oEmbed provider support.
- Support for Instagram share URLs.
- Preview debug page showing the generated metadata.
- Prometheus metrics.
- Configurable branding/name.
- A browser extension or mobile shortcut.

## 22. Main Risks

### Instagram scraping brittleness

Instagram can change embed HTML, script data, GraphQL document IDs, headers, CDN URL behavior, or login requirements at any time.

Mitigation:

- Keep scraping code isolated.
- Use fixtures.
- Cache aggressively but not permanently.
- Accept graceful fallback previews.
- Keep official oEmbed support as a future provider if credentials become available.

### Discord cache confusion

Discord caches link previews. A fixed bug may not appear fixed immediately for the same URL.

Mitigation:

- Use unique test shortcodes/URLs when possible.
- Add `?preview=1` for local HTML debugging.
- Include a debug route later if needed.

### Video support complexity

Video embeds can require correct content type, range requests, stable URLs, and Discord-specific behavior.

Mitigation:

- MVP success is image preview.
- Add video streaming as an enhancement.
- Fallback to poster image when video fails.

### Legal and platform policy risk

The scrape fallback uses public web pages and undocumented structures. This can violate platform expectations or break without notice.

Mitigation:

- Fetch only user-submitted public post URLs.
- Do not store permanent media archives.
- Do not bypass private/login-only restrictions.
- Prefer official APIs if available.

## 23. Implementation Phases

### Phase 1: Clean skeleton

- New Go app structure.
- Config loading.
- Nginx and Docker Compose.
- SQLite open/migrate.
- `/healthz`.
- Request logging.

### Phase 2: URL converter

- URL normalization package.
- `/api/convert`.
- Home UI.
- Copy button.
- Unit tests for normalization.

### Phase 3: Embed rendering

- Crawler detection.
- Human redirect.
- Metadata HTML template.
- Fallback metadata.
- Unit tests for rendering and crawler detection.

### Phase 4: Instagram scraper and cache

- Embed page fetcher.
- Parser using fixtures.
- SQLite cache.
- Singleflight duplicate request suppression.
- Negative cache.

### Phase 5: Media endpoints

- Image media endpoint.
- Redirect mode.
- Optional stream mode.
- Content type handling.

### Phase 6: VPS deployment hardening

- Nginx rate limits.
- TLS documentation.
- `.env.example`.
- README deployment section.
- Manual Discord test.

## 24. Recommended MVP Defaults

```text
Language: Go
Router: standard net/http ServeMux
Templates: html/template
Database: SQLite
SQLite driver: modernc.org/sqlite
Frontend: server-rendered HTML plus tiny vanilla JS
Deployment: Docker Compose
Reverse proxy: nginx:alpine container
Media mode: redirect
Video support: poster/image first, video streaming later
GraphQL fallback: disabled by default
```

## 25. Definition of Done

The rebuild is done when a public Discord channel can receive a generated link and display a useful embed, while clicking the same link opens the actual Instagram page.

Minimum proof:

1. A public Instagram image post embeds in Discord.
2. A public Instagram reel embeds with at least a poster image in Discord.
3. Browser clicks redirect to Instagram.
4. The app runs from Docker Compose on a VPS.
5. No endpoint acts as an arbitrary URL proxy.
