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
		Store:            store,
		Scraper:          fetcher,
	})
	if err != nil {
		t.Fatalf("NewHandlers() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/debug/p/ABC123xyz/refresh", nil)
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

func TestCanonicalUsesExpiredSuccessfulCacheWithoutRefetch(t *testing.T) {
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
	if fetcher.calls != 0 {
		t.Fatalf("fetch calls = %d, want 0", fetcher.calls)
	}
	if body := rr.Body.String(); !strings.Contains(body, "cached caption") || !strings.Contains(body, "preview/p/ABC123xyz/image") {
		t.Fatalf("embed did not use cached post:\n%s", body)
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
		targets[0] != "https://scontent.cdninstagram.com/poster.jpg" ||
		targets[1] != "https://scontent.cdninstagram.com/one.jpg" {
		t.Fatalf("targets = %#v", targets)
	}
}

func TestGalleryUsesConfiguredProfileAndLocalMediaURLs(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	if err := store.SaveAutomationConfig(ctx, "loonletwow", false, time.Now()); err != nil {
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
		!strings.Contains(body, `"imageUrl":"https://loonstagram.com/media/p/ABC123xyz/1/image"`) ||
		!strings.Contains(body, `"videoUrl":"https://loonstagram.com/media/p/ABC123xyz/2/video"`) {
		t.Fatalf("gallery response missing expected values:\n%s", body)
	}
	if strings.Contains(body, "scontent.cdninstagram.com") {
		t.Fatalf("gallery response should not expose upstream media URLs:\n%s", body)
	}
}

func TestRefreshGalleryFetchesRecentPosts(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	if err := store.SaveAutomationConfig(ctx, "loonletwow", false, time.Now()); err != nil {
		t.Fatalf("SaveAutomationConfig() error = %v", err)
	}
	ref := instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"}
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
	if body := rr.Body.String(); !strings.Contains(body, `"shortcode":"ABC123xyz"`) ||
		!strings.Contains(body, `"imageUrl":"https://loonstagram.com/media/p/ABC123xyz/1/image"`) {
		t.Fatalf("refresh response missing gallery item:\n%s", body)
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
	calls int
}

func (f *fakeProfileFetcher) FetchRecentMedia(ctx context.Context, username string, limit int) ([]instagram.RecentMedia, error) {
	f.calls++
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
