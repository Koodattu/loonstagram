package httpx

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"Loonstagram/internal/cache"
	"Loonstagram/internal/instagram"
	"Loonstagram/internal/mediacache"
	"Loonstagram/web"
)

func TestLoondokuRoute(t *testing.T) {
	templates, err := template.ParseFS(web.FS, "templates/*.html")
	if err != nil {
		t.Fatalf("ParseFS() error = %v", err)
	}
	h := &Handlers{templates: templates, logger: slog.Default()}
	req := httptest.NewRequest(http.MethodGet, "/loondoku", nil)
	rr := httptest.NewRecorder()

	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if contentType := rr.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q", contentType)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `id="loondoku-board"`) || !strings.Contains(body, `src="/static/loondoku.js`) {
		t.Fatalf("response does not contain the Loondoku page\n%s", body)
	}
}

func TestEmbedDataUsesUsernameCaptionThemeAndMultipleImages(t *testing.T) {
	h := &Handlers{publicBaseURL: "https://loonstagram.com"}
	post := &instagram.Post{
		Ref:      instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"},
		Username: "loonletwow",
		Caption:  "The squad coming at you like",
		Media: []instagram.MediaItem{
			{Kind: "image", URL: "https://scontent.cdninstagram.com/one.jpg", Width: 1080, Height: 1080},
			{Kind: "video", URL: "https://scontent.cdninstagram.com/two.mp4", PosterURL: "https://scontent.cdninstagram.com/two.jpg", Width: 720, Height: 1280},
		},
		Status: "ok",
	}

	data := h.embedData(post)
	if data.Title != "@loonletwow" {
		t.Fatalf("Title = %q", data.Title)
	}
	if data.Description != "The squad coming at you like" {
		t.Fatalf("Description = %q", data.Description)
	}
	if data.ThemeColor != "#d62976" {
		t.Fatalf("ThemeColor = %q", data.ThemeColor)
	}
	if !data.HasImage || data.ImageURL != "https://loonstagram.com/preview/p/ABC123xyz/image" {
		t.Fatalf("ImageURL = %q, HasImage = %v", data.ImageURL, data.HasImage)
	}
	if len(data.Images) != 2 {
		t.Fatalf("Images length = %d", len(data.Images))
	}
	if data.Images[0].URL != "https://loonstagram.com/media/p/ABC123xyz/1/image" ||
		data.Images[1].URL != "https://loonstagram.com/media/p/ABC123xyz/2/image" {
		t.Fatalf("Images = %#v", data.Images)
	}
}

func TestEmbedDataUsesFullCaption(t *testing.T) {
	h := &Handlers{publicBaseURL: "https://loonstagram.com"}
	longCaption := strings.Repeat("caption ", 80)
	post := &instagram.Post{
		Ref:      instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"},
		Username: "loonletwow",
		Caption:  longCaption,
		Status:   "ok",
	}

	data := h.embedData(post)
	if data.Description != strings.TrimSpace(longCaption) {
		t.Fatalf("Description was truncated: got %d chars, want %d", len(data.Description), len(strings.TrimSpace(longCaption)))
	}
}

func TestEmbedTemplateUsesSingleImageWithoutDimensions(t *testing.T) {
	templates, err := template.ParseFS(web.FS, "templates/embed.html")
	if err != nil {
		t.Fatalf("ParseFS() error = %v", err)
	}
	data := embedData{
		SiteName:    "Loonstagram",
		Title:       "@loonletwow",
		Description: "caption",
		OriginalURL: "https://www.instagram.com/p/ABC123xyz/",
		ThemeColor:  "#d62976",
		ImageURL:    "https://loonstagram.com/preview/p/ABC123xyz/image",
		Images: []embedImage{
			{URL: "https://loonstagram.com/media/p/ABC123xyz/1/image", Width: 1080, Height: 1080},
			{URL: "https://loonstagram.com/media/p/ABC123xyz/2/image", Width: 320, Height: 320},
		},
		HasImage: true,
	}

	var buf bytes.Buffer
	if err := templates.ExecuteTemplate(&buf, "embed.html", data); err != nil {
		t.Fatalf("ExecuteTemplate() error = %v", err)
	}
	html := buf.String()
	if got := strings.Count(html, `property="og:image"`); got != 1 {
		t.Fatalf("og:image count = %d, want 1\n%s", got, html)
	}
	if got := strings.Count(html, `name="twitter:image"`); got != 1 {
		t.Fatalf("twitter:image count = %d, want 1\n%s", got, html)
	}
	if strings.Contains(html, "og:image:width") ||
		strings.Contains(html, "og:image:height") ||
		strings.Contains(html, "twitter:image:width") ||
		strings.Contains(html, "twitter:image:height") {
		t.Fatalf("image dimension metadata should not be emitted\n%s", html)
	}
}

func TestEmbedDataUsesOriginalIndexForFirstUsablePreview(t *testing.T) {
	h := &Handlers{publicBaseURL: "https://loonstagram.com"}
	post := &instagram.Post{
		Ref: instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"},
		Media: []instagram.MediaItem{
			{Kind: "video", URL: "https://scontent.cdninstagram.com/one.mp4"},
			{Kind: "image", URL: "https://scontent.cdninstagram.com/two.jpg"},
		},
		Status: "ok",
	}

	data := h.embedData(post)
	if !data.HasImage || data.ImageURL != "https://loonstagram.com/preview/p/ABC123xyz/image" {
		t.Fatalf("ImageURL = %q, HasImage = %v", data.ImageURL, data.HasImage)
	}
}

func TestEmbedDataVersionsPreviewImageFromFetchedAt(t *testing.T) {
	h := &Handlers{publicBaseURL: "https://loonstagram.com"}
	post := &instagram.Post{
		Ref:       instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"},
		Media:     []instagram.MediaItem{{Kind: "image", URL: "https://scontent.cdninstagram.com/two.jpg"}},
		Status:    "ok",
		FetchedAt: time.Unix(1710000000, 0),
	}

	data := h.embedData(post)
	if !data.HasImage || data.ImageURL != "https://loonstagram.com/preview/p/ABC123xyz/image?v=1710000000" {
		t.Fatalf("ImageURL = %q, HasImage = %v", data.ImageURL, data.HasImage)
	}
}

func TestRefreshDebugCacheDeletesAndRefetchesPost(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	ref := instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"}
	now := time.Unix(1000, 0)
	if err := store.Put(ctx, &instagram.Post{
		Ref:       ref,
		Username:  "old",
		Status:    "ok",
		FetchedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	fetcher := &fakePostFetcher{
		post: &instagram.Post{
			Username: "new",
			Caption:  "caption",
			Media:    []instagram.MediaItem{{Kind: "image", URL: "https://scontent.cdninstagram.com/new.jpg"}},
		},
	}
	h, err := NewHandlers(Options{
		PublicBaseURL:    "https://loonstagram.com",
		CacheSuccessTTL:  time.Hour,
		CacheNegativeTTL: time.Minute,
		CacheBlockedTTL:  time.Minute,
		AdminToken:       "secret",
		Store:            store,
		Scraper:          fetcher,
	})
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/debug/p/ABC123xyz/refresh", nil)
	req.Header.Set("X-Admin-Token", "secret")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusSeeOther)
	}
	if location := rr.Header().Get("Location"); !strings.HasPrefix(location, "/debug/p/ABC123xyz?") {
		t.Fatalf("Location = %q", location)
	}
	if fetcher.calls != 1 {
		t.Fatalf("fetch calls = %d, want 1", fetcher.calls)
	}

	got, ok, err := store.Get(ctx, ref, time.Now())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("refreshed cache row missing")
	}
	if got.Username != "new" || len(got.Media) != 1 {
		t.Fatalf("cached post = %#v", got)
	}
}

func TestDebugRoutesRequireAdminTokenBeforeFetchingOrDeleting(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	ref := instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"}
	if err := store.Put(ctx, &instagram.Post{
		Ref:       ref,
		Username:  "cached",
		Media:     []instagram.MediaItem{{Kind: "image", URL: "https://scontent.cdninstagram.com/cached.jpg"}},
		Status:    "ok",
		FetchedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	fetcher := &fakePostFetcher{post: &instagram.Post{Username: "new"}}
	h, err := NewHandlers(Options{
		PublicBaseURL:    "https://loonstagram.com",
		CacheSuccessTTL:  time.Hour,
		CacheNegativeTTL: time.Minute,
		CacheBlockedTTL:  time.Minute,
		AdminToken:       "secret",
		Store:            store,
		Scraper:          fetcher,
	})
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}

	for _, test := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/debug/p/ABC123xyz"},
		{method: http.MethodPost, path: "/debug/p/ABC123xyz/refresh"},
	} {
		req := httptest.NewRequest(test.method, test.path, nil)
		rr := httptest.NewRecorder()
		h.Routes().ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("%s %s status = %d, want %d", test.method, test.path, rr.Code, http.StatusForbidden)
		}
		if cacheControl := rr.Header().Get("Cache-Control"); cacheControl != "private, no-store" {
			t.Fatalf("Cache-Control = %q", cacheControl)
		}
	}
	if fetcher.calls != 0 {
		t.Fatalf("fetch calls = %d, want 0", fetcher.calls)
	}
	if post, ok, err := store.GetAny(ctx, ref); err != nil || !ok || post.Username != "cached" {
		t.Fatalf("cached post = %#v, ok = %v, err = %v", post, ok, err)
	}
}

func TestDebugSessionExchangesHeaderTokenForScopedCookie(t *testing.T) {
	store, err := cache.Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	h, err := NewHandlers(Options{
		PublicBaseURL: "https://loonstagram.com",
		AdminToken:    "secret",
		Store:         store,
		Scraper:       &fakePostFetcher{},
	})
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/debug/session", nil)
	req.Header.Set("X-Admin-Token", "secret")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	cookie := rr.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, debugAuthCookieName+"=") || !strings.Contains(cookie, "Path=/debug") ||
		!strings.Contains(cookie, "HttpOnly") || !strings.Contains(cookie, "Secure") || !strings.Contains(cookie, "SameSite=Strict") {
		t.Fatalf("Set-Cookie = %q", cookie)
	}

	queryRequest := httptest.NewRequest(http.MethodGet, "/debug/p/ABC123xyz?admin_token=secret", nil)
	queryRecorder := httptest.NewRecorder()
	h.Routes().ServeHTTP(queryRecorder, queryRequest)
	if queryRecorder.Code != http.StatusForbidden {
		t.Fatalf("query token status = %d, want %d", queryRecorder.Code, http.StatusForbidden)
	}

	expiredRequest := httptest.NewRequest(http.MethodGet, "/debug/p/ABC123xyz", nil)
	expiredRequest.AddCookie(&http.Cookie{Name: debugAuthCookieName, Value: h.debugAuthCookieValue(time.Now().Add(-time.Minute))})
	expiredRecorder := httptest.NewRecorder()
	h.Routes().ServeHTTP(expiredRecorder, expiredRequest)
	if expiredRecorder.Code != http.StatusForbidden {
		t.Fatalf("expired cookie status = %d, want %d", expiredRecorder.Code, http.StatusForbidden)
	}
}

func TestCanonicalStripsTrailingSlashBeforeRouteMatch(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	h, err := NewHandlers(Options{
		PublicBaseURL:    "https://loonstagram.com",
		CacheSuccessTTL:  time.Hour,
		CacheNegativeTTL: time.Minute,
		CacheBlockedTTL:  time.Minute,
		Store:            store,
		Scraper: &fakePostFetcher{post: &instagram.Post{
			Username: "loonletwow",
			Caption:  "caption",
			Media:    []instagram.MediaItem{{Kind: "image", URL: "https://scontent.cdninstagram.com/post.jpg"}},
		}},
	})
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/p/ABC123xyz/", nil)
	req.Header.Set("User-Agent", "Discordbot/2.0")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "preview/p/ABC123xyz/image") {
		t.Fatalf("embed response did not use stripped path:\n%s", rr.Body.String())
	}
}

func TestCanonicalUsesExpiredSuccessfulCacheWhenRefreshFails(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	ref := instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"}
	now := time.Unix(1000, 0)
	if err := store.Put(ctx, &instagram.Post{
		Ref:         ref,
		OriginalURL: ref.OriginalURL(),
		Username:    "loonletwow",
		Caption:     "cached caption",
		Media:       []instagram.MediaItem{{Kind: "image", URL: "https://scontent.cdninstagram.com/cached.jpg"}},
		Status:      "ok",
		FetchedAt:   now,
		ExpiresAt:   now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	fetcher := &fakePostFetcher{err: errors.New("should not fetch")}
	h, err := NewHandlers(Options{
		PublicBaseURL:    "https://loonstagram.com",
		CacheSuccessTTL:  time.Hour,
		CacheNegativeTTL: time.Minute,
		CacheBlockedTTL:  time.Minute,
		Store:            store,
		Scraper:          fetcher,
	})
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/p/ABC123xyz", nil)
	req.Header.Set("User-Agent", "Discordbot/2.0")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if fetcher.calls != 1 {
		t.Fatalf("fetch calls = %d, want 1", fetcher.calls)
	}
	if body := rr.Body.String(); !strings.Contains(body, "cached caption") || !strings.Contains(body, "preview/p/ABC123xyz/image") {
		t.Fatalf("embed did not use cached post:\n%s", body)
	}

	second := httptest.NewRequest(http.MethodGet, "/p/ABC123xyz", nil)
	second.Header.Set("User-Agent", "Discordbot/2.0")
	secondRecorder := httptest.NewRecorder()
	h.Routes().ServeHTTP(secondRecorder, second)
	if secondRecorder.Code != http.StatusOK {
		t.Fatalf("second status = %d, want %d", secondRecorder.Code, http.StatusOK)
	}
	if fetcher.calls != 1 {
		t.Fatalf("fetch calls after stale retry window = %d, want 1", fetcher.calls)
	}
}

func TestCanonicalRefreshesExpiredSuccessfulCache(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	ref := instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"}
	if err := store.Put(ctx, &instagram.Post{
		Ref:       ref,
		Username:  "old",
		Caption:   "old caption",
		Media:     []instagram.MediaItem{{Kind: "image", URL: "https://scontent.cdninstagram.com/old.jpg"}},
		Status:    "ok",
		FetchedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	fetcher := &fakePostFetcher{post: &instagram.Post{
		Username: "new",
		Caption:  "new caption",
		Media:    []instagram.MediaItem{{Kind: "image", URL: "https://scontent.cdninstagram.com/new.jpg"}},
	}}
	h, err := NewHandlers(Options{
		PublicBaseURL:    "https://loonstagram.com",
		CacheSuccessTTL:  time.Hour,
		CacheNegativeTTL: time.Minute,
		CacheBlockedTTL:  time.Minute,
		Store:            store,
		Scraper:          fetcher,
	})
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/p/ABC123xyz", nil)
	req.Header.Set("User-Agent", "Discordbot/2.0")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if fetcher.calls != 1 {
		t.Fatalf("fetch calls = %d, want 1", fetcher.calls)
	}
	if body := rr.Body.String(); !strings.Contains(body, "new caption") || strings.Contains(body, "old caption") {
		t.Fatalf("embed did not use refreshed post:\n%s", body)
	}
	if post, ok, err := store.Get(ctx, ref, time.Now()); err != nil || !ok || post.Username != "new" {
		t.Fatalf("refreshed post = %#v, ok = %v, err = %v", post, ok, err)
	}
}

func TestPreviewImageJPEGUsesAdaptiveSingleImageSize(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 300, 900))
	for y := 0; y < 900; y++ {
		for x := 0; x < 300; x++ {
			source.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 180, A: 255})
		}
	}

	body, err := previewImageJPEG([]image.Image{source})
	if err != nil {
		t.Fatalf("previewImageJPEG() error = %v", err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("jpeg.Decode() error = %v", err)
	}
	if decoded.Bounds().Dx() != 675 || decoded.Bounds().Dy() != discordPreviewMaxSize {
		t.Fatalf("decoded size = %dx%d", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}

func TestPreviewImageJPEGUsesCompactSquareForCarousel(t *testing.T) {
	sources := []image.Image{
		solidImage(200, 200, color.RGBA{R: 255, A: 255}),
		solidImage(200, 400, color.RGBA{G: 255, A: 255}),
		solidImage(400, 200, color.RGBA{B: 255, A: 255}),
		solidImage(300, 300, color.RGBA{R: 255, G: 255, A: 255}),
	}

	body, err := previewImageJPEG(sources)
	if err != nil {
		t.Fatalf("previewImageJPEG() error = %v", err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("jpeg.Decode() error = %v", err)
	}
	if decoded.Bounds().Dx() != discordPreviewMaxSize || decoded.Bounds().Dy() != discordPreviewMaxSize {
		t.Fatalf("decoded size = %dx%d", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}

func TestPreviewImageTargetsUsesImageAndVideoPosters(t *testing.T) {
	targets := previewImageTargets([]instagram.MediaItem{
		{Kind: "video", URL: "https://scontent.cdninstagram.com/video.mp4", PosterURL: "https://scontent.cdninstagram.com/poster.jpg"},
		{Kind: "image", URL: "https://scontent.cdninstagram.com/one.jpg"},
		{Kind: "image", URL: "javascript:alert(1)"},
	}, 2)

	if len(targets) != 2 ||
		targets[0].Index != 1 || targets[0].URL != "https://scontent.cdninstagram.com/poster.jpg" ||
		targets[1].Index != 2 || targets[1].URL != "https://scontent.cdninstagram.com/one.jpg" {
		t.Fatalf("targets = %#v", targets)
	}
}

func TestPreviewImageUsesCachedSourcesAfterUpstreamURLsExpire(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	ref := instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"}
	urls := []string{
		"https://scontent.cdninstagram.com/one.jpg?signed=old",
		"https://scontent.cdninstagram.com/two.jpg?signed=old",
	}
	if err := store.Put(ctx, &instagram.Post{
		Ref:      ref,
		Username: "loonletwow",
		Media: []instagram.MediaItem{
			{Kind: "image", URL: urls[0], ContentType: "image/jpeg"},
			{Kind: "image", URL: urls[1], ContentType: "image/jpeg"},
		},
		Status:    "ok",
		FetchedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	mediaCache, err := mediacache.Open(t.TempDir(), 4*1024*1024)
	if err != nil {
		t.Fatalf("mediacache.Open() error = %v", err)
	}
	for i, target := range urls {
		var body bytes.Buffer
		if err := jpeg.Encode(&body, solidImage(40+i*10, 40, color.RGBA{R: uint8(100 + i), A: 255}), nil); err != nil {
			t.Fatalf("jpeg.Encode() error = %v", err)
		}
		if _, err := mediaCache.Put(ctx, mediaCacheKey(ref, i+1, "image", target), "image/jpeg", bytes.NewReader(body.Bytes())); err != nil {
			t.Fatalf("mediaCache.Put() error = %v", err)
		}
	}

	upstreamCalls := 0
	h, err := NewHandlers(Options{
		PublicBaseURL:    "https://loonstagram.com",
		CacheSuccessTTL:  time.Hour,
		CacheNegativeTTL: time.Minute,
		CacheBlockedTTL:  time.Minute,
		Store:            store,
		MediaCache:       mediaCache,
		Scraper:          &fakePostFetcher{},
	})
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}
	h.mediaClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		upstreamCalls++
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("expired")),
			Request:    req,
		}, nil
	})}

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/preview/p/ABC123xyz/image", nil)
		rr := httptest.NewRecorder()
		h.Routes().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "image/jpeg" {
			t.Fatalf("request %d status = %d, content type = %q", i+1, rr.Code, rr.Header().Get("Content-Type"))
		}
	}
	if upstreamCalls != 0 {
		t.Fatalf("upstream calls = %d, want 0", upstreamCalls)
	}
}

func TestSinglePreviewCachesUpstreamBeforeURLExpires(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	ref := instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"}
	target := "https://scontent.cdninstagram.com/post.jpg?signed=short-lived"
	if err := store.Put(ctx, &instagram.Post{
		Ref:       ref,
		Username:  "loonletwow",
		Media:     []instagram.MediaItem{{Kind: "image", URL: target, ContentType: "image/jpeg"}},
		Status:    "ok",
		FetchedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	mediaCache, err := mediacache.Open(t.TempDir(), 4*1024*1024)
	if err != nil {
		t.Fatalf("mediacache.Open() error = %v", err)
	}
	var imageBody bytes.Buffer
	if err := jpeg.Encode(&imageBody, solidImage(80, 60, color.RGBA{R: 120, G: 80, A: 255}), nil); err != nil {
		t.Fatalf("jpeg.Encode() error = %v", err)
	}
	upstreamAvailable := true
	upstreamCalls := 0
	h, err := NewHandlers(Options{
		PublicBaseURL:    "https://loonstagram.com",
		CacheSuccessTTL:  time.Hour,
		CacheNegativeTTL: time.Minute,
		CacheBlockedTTL:  time.Minute,
		Store:            store,
		MediaCache:       mediaCache,
		Scraper:          &fakePostFetcher{},
	})
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}
	h.mediaClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		upstreamCalls++
		status := http.StatusOK
		body := imageBody.String()
		if !upstreamAvailable {
			status = http.StatusForbidden
			body = "expired"
		}
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{"image/jpeg"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/preview/p/ABC123xyz/image", nil)
		rr := httptest.NewRecorder()
		h.Routes().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "image/jpeg" {
			t.Fatalf("request %d status = %d, content type = %q", i+1, rr.Code, rr.Header().Get("Content-Type"))
		}
		upstreamAvailable = false
	}
	if upstreamCalls != 1 {
		t.Fatalf("upstream calls = %d, want 1", upstreamCalls)
	}
}

func TestGalleryUsesConfiguredProfileAndLocalMediaURLs(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	if err := store.SaveAutomationConfig(ctx, "loonletwow", false, 30, time.Now()); err != nil {
		t.Fatalf("SaveAutomationConfig() error = %v", err)
	}
	ref := instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"}
	now := time.Unix(1710000000, 0)
	if err := store.Put(ctx, &instagram.Post{
		Ref:         ref,
		OriginalURL: ref.OriginalURL(),
		Username:    "loonletwow",
		Caption:     "caption",
		Media: []instagram.MediaItem{
			{Kind: "image", URL: "https://scontent.cdninstagram.com/one.jpg", Width: 1080, Height: 1080},
			{Kind: "video", URL: "https://scontent.cdninstagram.com/two.mp4", PosterURL: "https://scontent.cdninstagram.com/two.jpg"},
		},
		Status:    "ok",
		FetchedAt: now,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	h, err := NewHandlers(Options{
		PublicBaseURL:    "https://loonstagram.com",
		CacheSuccessTTL:  time.Hour,
		CacheNegativeTTL: time.Minute,
		CacheBlockedTTL:  time.Minute,
		Store:            store,
		Scraper:          &fakePostFetcher{},
	})
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gallery", nil)
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"profile":"loonletwow"`) ||
		!strings.Contains(body, `"canonicalUrl":"https://loonstagram.com/p/ABC123xyz"`) ||
		!strings.Contains(body, `"imageUrl":"https://loonstagram.com/gallery-media/p/ABC123xyz/1/image"`) ||
		!strings.Contains(body, `"videoUrl":"https://loonstagram.com/gallery-media/p/ABC123xyz/2/video"`) {
		t.Fatalf("gallery response missing expected values:\n%s", body)
	}
	if strings.Contains(body, "scontent.cdninstagram.com") {
		t.Fatalf("gallery response should not expose upstream media URLs:\n%s", body)
	}
}

func TestRefreshGalleryRefreshesExpiredRecentPosts(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	if err := store.SaveAutomationConfig(ctx, "loonletwow", false, 30, time.Now()); err != nil {
		t.Fatalf("SaveAutomationConfig() error = %v", err)
	}
	ref := instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"}
	if err := store.Put(ctx, &instagram.Post{
		Ref:       ref,
		Username:  "old",
		Caption:   "old caption",
		Media:     []instagram.MediaItem{{Kind: "image", URL: "https://scontent.cdninstagram.com/old.jpg"}},
		Status:    "ok",
		FetchedAt: time.Now().Add(-2 * time.Hour),
		ExpiresAt: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	profiles := &fakeProfileFetcher{media: []instagram.RecentMedia{{
		Ref:          ref,
		Username:     "loonletwow",
		InstagramURL: ref.OriginalURL(),
	}}}
	fetcher := &fakePostFetcher{post: &instagram.Post{
		Username: "loonletwow",
		Caption:  "caption",
		Media:    []instagram.MediaItem{{Kind: "image", URL: "https://scontent.cdninstagram.com/post.jpg"}},
	}}
	h, err := NewHandlers(Options{
		PublicBaseURL:    "https://loonstagram.com",
		CacheSuccessTTL:  time.Hour,
		CacheNegativeTTL: time.Minute,
		CacheBlockedTTL:  time.Minute,
		AdminToken:       "secret",
		Store:            store,
		Scraper:          fetcher,
		Profiles:         profiles,
	})
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/gallery/refresh", nil)
	req.Header.Set("X-Admin-Token", "secret")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if profiles.calls != 1 {
		t.Fatalf("profile fetch calls = %d, want 1", profiles.calls)
	}
	if fetcher.calls != 1 {
		t.Fatalf("post fetch calls = %d, want 1", fetcher.calls)
	}
	if post, ok, err := store.Get(ctx, ref, time.Now()); err != nil || !ok || post.Caption != "caption" {
		t.Fatalf("refreshed post = %#v, ok = %v, err = %v", post, ok, err)
	}
	if body := rr.Body.String(); !strings.Contains(body, `"shortcode":"ABC123xyz"`) ||
		!strings.Contains(body, `"imageUrl":"https://loonstagram.com/gallery-media/p/ABC123xyz/1/image"`) {
		t.Fatalf("refresh response missing gallery item:\n%s", body)
	}
}

func TestRefreshGalleryCooldownAfterBlockedProfileFetch(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	if err := store.SaveAutomationConfig(ctx, "loonletwow", false, 30, time.Now()); err != nil {
		t.Fatalf("SaveAutomationConfig() error = %v", err)
	}
	profiles := &fakeProfileFetcher{
		err: instagram.ProfileFetchError{Kind: instagram.FetchErrorBlocked, Message: "instagram profile fetch blocked"},
	}
	h, err := NewHandlers(Options{
		PublicBaseURL:    "https://loonstagram.com",
		CacheSuccessTTL:  time.Hour,
		CacheNegativeTTL: time.Minute,
		CacheBlockedTTL:  time.Minute,
		AdminToken:       "secret",
		Store:            store,
		Scraper:          &fakePostFetcher{},
		Profiles:         profiles,
	})
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}

	first := httptest.NewRequest(http.MethodPost, "/api/gallery/refresh", nil)
	first.Header.Set("X-Admin-Token", "secret")
	firstRecorder := httptest.NewRecorder()
	h.Routes().ServeHTTP(firstRecorder, first)

	if firstRecorder.Code != http.StatusBadGateway {
		t.Fatalf("first status = %d, want %d: %s", firstRecorder.Code, http.StatusBadGateway, firstRecorder.Body.String())
	}
	if profiles.calls != 1 {
		t.Fatalf("profile fetch calls after first request = %d, want 1", profiles.calls)
	}

	second := httptest.NewRequest(http.MethodPost, "/api/gallery/refresh", nil)
	second.Header.Set("X-Admin-Token", "secret")
	secondRecorder := httptest.NewRecorder()
	h.Routes().ServeHTTP(secondRecorder, second)

	if secondRecorder.Code != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want %d: %s", secondRecorder.Code, http.StatusTooManyRequests, secondRecorder.Body.String())
	}
	if profiles.calls != 1 {
		t.Fatalf("profile fetch calls after cooldown request = %d, want 1", profiles.calls)
	}
	if !strings.Contains(secondRecorder.Body.String(), "cooling down") {
		t.Fatalf("second response should explain cooldown:\n%s", secondRecorder.Body.String())
	}
}

func TestMediaEndpointCachesUpstreamBytes(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	ref := instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"}
	if err := store.Put(ctx, &instagram.Post{
		Ref:         ref,
		OriginalURL: ref.OriginalURL(),
		Username:    "loonletwow",
		Media: []instagram.MediaItem{
			{Kind: "image", URL: "https://scontent.cdninstagram.com/one.jpg", ContentType: "image/jpeg"},
		},
		Status:    "ok",
		FetchedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	mediaCache, err := mediacache.Open(t.TempDir(), 1024)
	if err != nil {
		t.Fatalf("mediacache.Open() error = %v", err)
	}
	upstreamCalls := 0
	h, err := NewHandlers(Options{
		PublicBaseURL:    "https://loonstagram.com",
		CacheSuccessTTL:  time.Hour,
		CacheNegativeTTL: time.Minute,
		CacheBlockedTTL:  time.Minute,
		MediaProxyMode:   "stream",
		Store:            store,
		MediaCache:       mediaCache,
		Scraper:          &fakePostFetcher{},
	})
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}
	h.mediaClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		upstreamCalls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/jpeg"}},
			Body:       io.NopCloser(strings.NewReader("cached image")),
			Request:    req,
		}, nil
	})}

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/media/p/ABC123xyz/1/image", nil)
		rr := httptest.NewRecorder()
		h.Routes().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want %d: %s", i+1, rr.Code, http.StatusOK, rr.Body.String())
		}
		if body := rr.Body.String(); body != "cached image" {
			t.Fatalf("request %d body = %q", i+1, body)
		}
	}
	if upstreamCalls != 1 {
		t.Fatalf("upstream calls = %d, want 1", upstreamCalls)
	}
}

func TestMediaEndpointStreamsUncachedVideoRange(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	ref := instagram.Ref{Type: instagram.TypeReel, Shortcode: "ABC123xyz"}
	target := "https://scontent.cdninstagram.com/video.mp4"
	if err := store.Put(ctx, &instagram.Post{
		Ref:       ref,
		Username:  "loonletwow",
		Media:     []instagram.MediaItem{{Kind: "video", URL: target, ContentType: "video/mp4"}},
		Status:    "ok",
		FetchedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	mediaCache, err := mediacache.Open(t.TempDir(), 1024)
	if err != nil {
		t.Fatalf("mediacache.Open() error = %v", err)
	}
	h, err := NewHandlers(Options{
		PublicBaseURL:  "https://loonstagram.com",
		MediaProxyMode: "stream",
		Store:          store,
		MediaCache:     mediaCache,
		Scraper:        &fakePostFetcher{},
	})
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}
	h.mediaClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodHead {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type":   []string{"video/mp4"},
					"Content-Length": []string{"10"},
					"Accept-Ranges":  []string{"bytes"},
				},
				Body:    io.NopCloser(strings.NewReader("must not be copied")),
				Request: req,
			}, nil
		}
		if req.Header.Get("Range") == "bytes=100-" {
			return &http.Response{
				StatusCode: http.StatusRequestedRangeNotSatisfiable,
				Header: http.Header{
					"Content-Range": []string{"bytes */10"},
					"Accept-Ranges": []string{"bytes"},
				},
				Body:    io.NopCloser(strings.NewReader("")),
				Request: req,
			}, nil
		}
		if got := req.Header.Get("Range"); got != "bytes=2-5" {
			t.Fatalf("upstream Range = %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Header: http.Header{
				"Content-Type":   []string{"video/mp4"},
				"Content-Length": []string{"4"},
				"Content-Range":  []string{"bytes 2-5/10"},
				"Accept-Ranges":  []string{"bytes"},
			},
			Body:    io.NopCloser(strings.NewReader("2345")),
			Request: req,
		}, nil
	})}
	h.streamClient = h.mediaClient

	req := httptest.NewRequest(http.MethodGet, "/media/reel/ABC123xyz/1/video", nil)
	req.Header.Set("Range", "bytes=2-5")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusPartialContent || rr.Body.String() != "2345" {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Content-Range") != "bytes 2-5/10" || rr.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("range headers = %#v", rr.Header())
	}

	head := httptest.NewRequest(http.MethodHead, "/media/reel/ABC123xyz/1/video", nil)
	headRecorder := httptest.NewRecorder()
	h.Routes().ServeHTTP(headRecorder, head)
	if headRecorder.Code != http.StatusOK || headRecorder.Body.Len() != 0 || headRecorder.Header().Get("Content-Length") != "10" {
		t.Fatalf("HEAD status = %d, body bytes = %d, Content-Length = %q", headRecorder.Code, headRecorder.Body.Len(), headRecorder.Header().Get("Content-Length"))
	}

	unsatisfied := httptest.NewRequest(http.MethodGet, "/media/reel/ABC123xyz/1/video", nil)
	unsatisfied.Header.Set("Range", "bytes=100-")
	unsatisfiedRecorder := httptest.NewRecorder()
	h.Routes().ServeHTTP(unsatisfiedRecorder, unsatisfied)
	if unsatisfiedRecorder.Code != http.StatusRequestedRangeNotSatisfiable || unsatisfiedRecorder.Header().Get("Content-Range") != "bytes */10" {
		t.Fatalf("unsatisfied status = %d, Content-Range = %q", unsatisfiedRecorder.Code, unsatisfiedRecorder.Header().Get("Content-Range"))
	}
}

func TestMediaEndpointServesCachedRangeBeforeRedirect(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	ref := instagram.Ref{Type: instagram.TypeReel, Shortcode: "ABC123xyz"}
	target := "https://scontent.cdninstagram.com/video.mp4"
	if err := store.Put(ctx, &instagram.Post{
		Ref:       ref,
		Username:  "loonletwow",
		Media:     []instagram.MediaItem{{Kind: "video", URL: target, ContentType: "video/mp4"}},
		Status:    "ok",
		FetchedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	mediaCache, err := mediacache.Open(t.TempDir(), 1024)
	if err != nil {
		t.Fatalf("mediacache.Open() error = %v", err)
	}
	if _, err := mediaCache.Put(ctx, mediaCacheKey(ref, 1, "video", target), "video/mp4", strings.NewReader("0123456789")); err != nil {
		t.Fatalf("mediaCache.Put() error = %v", err)
	}
	h, err := NewHandlers(Options{
		PublicBaseURL:  "https://loonstagram.com",
		MediaProxyMode: "redirect",
		Store:          store,
		MediaCache:     mediaCache,
		Scraper:        &fakePostFetcher{},
	})
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}
	h.mediaClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatal("cached media request contacted upstream")
		return nil, errors.New("unexpected upstream call")
	})}

	req := httptest.NewRequest(http.MethodGet, "/media/reel/ABC123xyz/1/video", nil)
	req.Header.Set("Range", "bytes=2-5")
	rr := httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusPartialContent || rr.Body.String() != "2345" {
		t.Fatalf("status = %d, body = %q", rr.Code, rr.Body.String())
	}

	uncachedTarget := "https://scontent.cdninstagram.com/new-video.mp4"
	if err := store.Put(ctx, &instagram.Post{
		Ref:       ref,
		Username:  "loonletwow",
		Media:     []instagram.MediaItem{{Kind: "video", URL: uncachedTarget, ContentType: "video/mp4"}},
		Status:    "ok",
		FetchedAt: time.Now().Add(time.Second),
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/media/reel/ABC123xyz/1/video", nil)
	rr = httptest.NewRecorder()
	h.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusFound || rr.Header().Get("Location") != uncachedTarget {
		t.Fatalf("uncached status = %d, Location = %q", rr.Code, rr.Header().Get("Location"))
	}
}

func TestMediaCacheKeyIncludesUpstreamTarget(t *testing.T) {
	ref := instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"}
	first := mediaCacheKey(ref, 1, "image", "https://scontent.cdninstagram.com/cropped.jpg")
	second := mediaCacheKey(ref, 1, "image", "https://scontent.cdninstagram.com/full.jpg")
	if first == second {
		t.Fatalf("media cache key should change when upstream target changes: %q", first)
	}
	if !strings.HasPrefix(first, "p_ABC123xyz_1_image_") {
		t.Fatalf("media cache key prefix = %q", first)
	}
}

func TestDebugCandidatesMarksSelectedMedia(t *testing.T) {
	h := &Handlers{}
	selectedURL := "https://scontent.cdninstagram.com/full.jpg?stp=dst-jpg_e35_s1080x1080_tt6"
	report := instagram.DebugReport{
		Fetches: []instagram.DebugFetch{{
			Name: "original_page",
			ExtractedJSON: []instagram.DebugJSONBlock{{
				Key:   "items",
				Index: 1,
				Raw: `[
					{
						"image_versions2": {
							"candidates": [
								{"url": "https://scontent.cdninstagram.com/cropped.jpg?stp=c288.0.864.864a_dst-jpg_e35_s640x640_tt6", "width": 864, "height": 864},
								{"url": "https://scontent.cdninstagram.com/full.jpg?stp=dst-jpg_e35_s1080x1080_tt6", "width": 1080, "height": 1080}
							]
						}
					}
				]`,
			}},
		}},
	}
	post := &instagram.Post{
		Media: []instagram.MediaItem{{Kind: "image", URL: selectedURL}},
	}

	candidates := h.debugCandidates(report, post)
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2: %#v", len(candidates), candidates)
	}
	if candidates[0].Selected || !candidates[0].Cropped {
		t.Fatalf("first candidate = %#v", candidates[0])
	}
	if !candidates[1].Selected || candidates[1].Cropped {
		t.Fatalf("second candidate = %#v", candidates[1])
	}
	if candidates[1].Role != "selected media" {
		t.Fatalf("second candidate role = %q", candidates[1].Role)
	}
}

func TestDebugCandidateRoleClassifiesPostAndProfileImages(t *testing.T) {
	selectedFilenames := map[string]bool{
		"619289718_17951341275072694_8657305568275949427_n.jpg": true,
	}
	postCandidate := "https://scontent.cdninstagram.com/v/t51.82787-15/619289718_17951341275072694_8657305568275949427_n.jpg?stp=dst-jpg_e35_s1080x1080_tt6"
	profileCandidate := "https://scontent.cdninstagram.com/v/t51.2885-19/437590353_2192875307721665_4063332443154026003_n.jpg?stp=dst-jpg_s150x150_tt6"

	if role := debugCandidateRole(postCandidate, false, selectedFilenames); role != "post media" {
		t.Fatalf("post candidate role = %q", role)
	}
	if role := debugCandidateRole(profileCandidate, false, selectedFilenames); role != "profile image" {
		t.Fatalf("profile candidate role = %q", role)
	}
}

func solidImage(width, height int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type fakePostFetcher struct {
	post  *instagram.Post
	err   error
	calls int
}

func (f *fakePostFetcher) FetchPost(ctx context.Context, ref instagram.Ref) (*instagram.Post, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if f.post == nil {
		return &instagram.Post{Ref: ref, Status: "ok"}, nil
	}
	post := *f.post
	post.Ref = ref
	return &post, nil
}

type fakeProfileFetcher struct {
	media []instagram.RecentMedia
	err   error
	calls int
}

func (f *fakeProfileFetcher) FetchRecentMedia(ctx context.Context, username string, limit int) ([]instagram.RecentMedia, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.media, nil
}

func TestShouldRefreshCachedPost(t *testing.T) {
	tests := []struct {
		name string
		post *instagram.Post
		want bool
	}{
		{
			name: "complete ok post",
			post: &instagram.Post{
				Status:   "ok",
				Username: "loonletwow",
				Caption:  "caption",
				Media:    []instagram.MediaItem{{Kind: "image", URL: "https://scontent.cdninstagram.com/post.jpg"}},
			},
			want: false,
		},
		{
			name: "old ok fallback with no metadata",
			post: &instagram.Post{
				Status: "ok",
				Media:  []instagram.MediaItem{{Kind: "image", URL: "https://scontent.cdninstagram.com/profile.jpg"}},
			},
			want: true,
		},
		{
			name: "ok post without media",
			post: &instagram.Post{
				Status:   "ok",
				Username: "loonletwow",
				Caption:  "caption",
			},
			want: true,
		},
		{
			name: "negative cache",
			post: &instagram.Post{Status: "blocked"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRefreshCachedPost(tt.post); got != tt.want {
				t.Fatalf("shouldRefreshCachedPost() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldRefreshGalleryPostRefreshesCroppedMedia(t *testing.T) {
	post := &instagram.Post{
		Status:   "ok",
		Username: "loonletwow",
		Media: []instagram.MediaItem{{
			Kind: "image",
			URL:  "https://scontent-hel3-1.cdninstagram.com/post.jpg?stp=c288.0.864.864a_dst-jpg_e35_s640x640_tt6",
		}},
	}
	if !shouldRefreshGalleryPost(post) {
		t.Fatal("cropped gallery media should be refreshed")
	}

	post.Media[0].URL = "https://scontent-hel3-1.cdninstagram.com/post.jpg?stp=dst-jpg_e35_s1080x1080_tt6"
	if shouldRefreshGalleryPost(post) {
		t.Fatal("complete uncropped gallery media should remain cached")
	}
}
