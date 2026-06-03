package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"instafix/internal/cache"
	"instafix/internal/instagram"
	"instafix/web"
)

type PostFetcher interface {
	FetchPost(ctx context.Context, ref instagram.Ref) (*instagram.Post, error)
}

type Options struct {
	PublicBaseURL    string
	CacheSuccessTTL  time.Duration
	CacheNegativeTTL time.Duration
	CacheBlockedTTL  time.Duration
	MediaProxyMode   string
	Store            *cache.Store
	Scraper          PostFetcher
	Logger           *slog.Logger
}

type Handlers struct {
	publicBaseURL    string
	cacheSuccessTTL  time.Duration
	cacheNegativeTTL time.Duration
	cacheBlockedTTL  time.Duration
	mediaProxyMode   string
	store            *cache.Store
	scraper          PostFetcher
	logger           *slog.Logger
	templates        *template.Template
	flight           *flight
	mediaClient      *http.Client
}

func NewHandlers(opts Options) (*Handlers, error) {
	if opts.Store == nil {
		return nil, errors.New("cache store is required")
	}
	if opts.Scraper == nil {
		return nil, errors.New("scraper is required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.MediaProxyMode == "" {
		opts.MediaProxyMode = "redirect"
	}
	templates, err := template.ParseFS(web.FS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Handlers{
		publicBaseURL:    strings.TrimRight(opts.PublicBaseURL, "/"),
		cacheSuccessTTL:  opts.CacheSuccessTTL,
		cacheNegativeTTL: opts.CacheNegativeTTL,
		cacheBlockedTTL:  opts.CacheBlockedTTL,
		mediaProxyMode:   opts.MediaProxyMode,
		store:            opts.Store,
		scraper:          opts.Scraper,
		logger:           opts.Logger,
		templates:        templates,
		flight:           newFlight(),
		mediaClient: &http.Client{
			Timeout: 20 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return errors.New("too many media redirects")
				}
				if !safeRemoteURL(req.URL.String()) {
					return errors.New("unsafe media redirect")
				}
				return nil
			},
		},
	}, nil
}

func (h *Handlers) Routes() http.Handler {
	mux := http.NewServeMux()
	staticFS := http.FS(web.StaticFS())
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(staticFS)))
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("GET /", h.home)
	mux.HandleFunc("POST /api/convert", h.convert)
	mux.HandleFunc("GET /p/{shortcode}", h.canonical(instagram.TypePost))
	mux.HandleFunc("GET /reel/{shortcode}", h.canonical(instagram.TypeReel))
	mux.HandleFunc("GET /tv/{shortcode}", h.canonical(instagram.TypeTV))
	mux.HandleFunc("GET /media/{type}/{shortcode}/{index}/image", h.media("image"))
	mux.HandleFunc("GET /media/{type}/{shortcode}/{index}/video", h.media("video"))
	return RequestLogger(h.logger, mux)
}

func (h *Handlers) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "home.html", map[string]string{
		"PublicBaseURL": h.publicBaseURL,
	}); err != nil {
		h.logger.Error("render home", "error", err)
	}
}

func (h *Handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type convertRequest struct {
	URL string `json:"url"`
}

type convertResponse struct {
	OK        bool   `json:"ok"`
	URL       string `json:"url,omitempty"`
	Type      string `json:"type,omitempty"`
	Shortcode string `json:"shortcode,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (h *Handlers) convert(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	defer r.Body.Close()

	var req convertRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, convertResponse{OK: false, Error: "Unsupported Instagram URL"})
		return
	}

	fixedURL, ref, err := instagram.ConvertURL(h.publicBaseURL, req.URL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, convertResponse{OK: false, Error: "Unsupported Instagram URL"})
		return
	}

	writeJSON(w, http.StatusOK, convertResponse{
		OK:        true,
		URL:       fixedURL,
		Type:      ref.Type,
		Shortcode: ref.Shortcode,
	})
}

func (h *Handlers) canonical(mediaType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref, err := instagram.NewRef(mediaType, r.PathValue("shortcode"))
		if err != nil {
			http.NotFound(w, r)
			return
		}

		if !IsCrawler(r) {
			http.Redirect(w, r, ref.OriginalURL(), http.StatusFound)
			return
		}

		post, err := h.getOrFetchPost(r.Context(), ref)
		if err != nil {
			h.logger.Warn("metadata fetch failed", "shortcode", ref.Shortcode, "media_type", ref.Type, "error", err)
			post = instagram.FallbackPost(ref, "error", "metadata fetch failed")
		}
		h.renderEmbed(w, post)
	}
}

func (h *Handlers) renderEmbed(w http.ResponseWriter, post *instagram.Post) {
	data := h.embedData(post)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "embed.html", data); err != nil {
		h.logger.Error("render embed", "shortcode", post.Ref.Shortcode, "error", err)
	}
}

type embedData struct {
	SiteName       string
	Title          string
	Description    string
	OriginalURL    string
	CanonicalURL   string
	ImageURL       string
	VideoURL       string
	HasImage       bool
	HasVideo       bool
	VideoType      string
	Username       string
	Shortcode      string
	MediaType      string
	ProviderStatus string
}

func (h *Handlers) embedData(post *instagram.Post) embedData {
	title := "Instagram post"
	if post.Username != "" {
		title = "@" + post.Username + " on Instagram"
	}

	description := instagram.CaptionPreview(post.Caption, 280)
	if description == "" {
		description = "Open this Instagram post."
	}

	data := embedData{
		SiteName:       "InstaFix",
		Title:          title,
		Description:    description,
		OriginalURL:    post.Ref.OriginalURL(),
		CanonicalURL:   h.publicURL(post.Ref.CanonicalPath()),
		Username:       post.Username,
		Shortcode:      post.Ref.Shortcode,
		MediaType:      post.Ref.Type,
		ProviderStatus: post.Status,
	}

	if post.OriginalURL != "" {
		data.OriginalURL = post.OriginalURL
	}

	if post.Status != "ok" {
		return data
	}

	first := firstPreviewMedia(post.Media)
	if first == nil {
		return data
	}

	if imageCandidate(*first) != "" {
		data.ImageURL = h.publicURL(fmt.Sprintf("/media/%s/%s/1/image", post.Ref.Type, post.Ref.Shortcode))
		data.HasImage = true
	}
	if first.Kind == "video" && first.URL != "" {
		data.VideoURL = h.publicURL(fmt.Sprintf("/media/%s/%s/1/video", post.Ref.Type, post.Ref.Shortcode))
		data.VideoType = firstString(first.ContentType, "video/mp4")
		data.HasVideo = true
	}

	return data
}

func (h *Handlers) media(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref, err := instagram.NewRef(r.PathValue("type"), r.PathValue("shortcode"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil || index < 1 {
			http.NotFound(w, r)
			return
		}

		post, err := h.getOrFetchPost(r.Context(), ref)
		if err != nil || post.Status != "ok" {
			http.NotFound(w, r)
			return
		}
		if index > len(post.Media) {
			http.NotFound(w, r)
			return
		}

		item := post.Media[index-1]
		target, contentType := mediaTarget(kind, item)
		if target == "" || !safeRemoteURL(target) {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Cache-Control", "public, max-age=300")
		if h.mediaProxyMode == "redirect" {
			http.Redirect(w, r, target, http.StatusFound)
			return
		}

		h.streamMedia(w, r, target, contentType)
	}
}

func (h *Handlers) getOrFetchPost(ctx context.Context, ref instagram.Ref) (*instagram.Post, error) {
	now := time.Now()
	if post, ok, err := h.store.Get(ctx, ref, now); ok || err != nil {
		if ok {
			setCacheStatus(ctx, "hit")
		}
		return post, err
	}

	setCacheStatus(ctx, "miss")
	key := ref.Type + ":" + ref.Shortcode
	return h.flight.Do(key, func() (*instagram.Post, error) {
		now := time.Now()
		if post, ok, err := h.store.Get(ctx, ref, now); ok || err != nil {
			if ok {
				setCacheStatus(ctx, "hit")
			}
			return post, err
		}

		start := time.Now()
		post, err := h.scraper.FetchPost(ctx, ref)
		if err != nil {
			status := "error"
			ttl := h.cacheNegativeTTL
			var fetchErr instagram.FetchError
			if errors.As(err, &fetchErr) {
				switch fetchErr.Kind {
				case instagram.FetchErrorBlocked:
					status = "blocked"
					ttl = h.cacheBlockedTTL
				case instagram.FetchErrorNotFound:
					status = "not_found"
				}
			}
			post = instagram.FallbackPost(ref, status, sanitizeLogError(err))
			post.FetchedAt = now
			post.ExpiresAt = now.Add(ttl)
			if putErr := h.store.Put(ctx, post); putErr != nil {
				return post, putErr
			}
			h.logger.Warn("scrape failed",
				"shortcode", ref.Shortcode,
				"media_type", ref.Type,
				"status", status,
				"duration_ms", time.Since(start).Milliseconds(),
				"error", sanitizeLogError(err),
			)
			return post, nil
		}

		post.Ref = ref
		post.Status = "ok"
		post.Error = ""
		post.FetchedAt = now
		post.ExpiresAt = now.Add(h.cacheSuccessTTL)
		if post.OriginalURL == "" {
			post.OriginalURL = ref.OriginalURL()
		}
		if putErr := h.store.Put(ctx, post); putErr != nil {
			return post, putErr
		}
		h.logger.Info("scrape complete",
			"shortcode", ref.Shortcode,
			"media_type", ref.Type,
			"provider", "embed_page",
			"duration_ms", time.Since(start).Milliseconds(),
		)
		return post, nil
	})
}

func (h *Handlers) streamMedia(w http.ResponseWriter, r *http.Request, target, fallbackContentType string) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		http.Error(w, "Bad upstream media URL", http.StatusBadGateway)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; InstaFix/1.0)")
	req.Header.Set("Accept", "*/*")
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	resp, err := h.mediaClient.Do(req)
	if err != nil {
		http.Error(w, "Media fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		http.Error(w, "Media fetch failed", http.StatusBadGateway)
		return
	}

	copyHeader(w.Header(), resp.Header, "Content-Type")
	copyHeader(w.Header(), resp.Header, "Content-Length")
	copyHeader(w.Header(), resp.Header, "Content-Range")
	copyHeader(w.Header(), resp.Header, "Accept-Ranges")
	if w.Header().Get("Content-Type") == "" && fallbackContentType != "" {
		w.Header().Set("Content-Type", fallbackContentType)
	}
	if resp.StatusCode == http.StatusPartialContent {
		w.WriteHeader(http.StatusPartialContent)
	}
	_, _ = io.Copy(w, resp.Body)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func copyHeader(dst http.Header, src http.Header, key string) {
	if value := src.Get(key); value != "" {
		dst.Set(key, value)
	}
}

func firstPreviewMedia(media []instagram.MediaItem) *instagram.MediaItem {
	for i := range media {
		if imageCandidate(media[i]) != "" {
			return &media[i]
		}
	}
	if len(media) == 0 {
		return nil
	}
	return &media[0]
}

func imageCandidate(item instagram.MediaItem) string {
	if item.Kind == "image" && item.URL != "" {
		return item.URL
	}
	return item.PosterURL
}

func mediaTarget(kind string, item instagram.MediaItem) (string, string) {
	switch kind {
	case "image":
		if item.Kind == "image" && item.URL != "" {
			return item.URL, firstString(item.ContentType, "image/jpeg")
		}
		if item.PosterURL != "" {
			return item.PosterURL, "image/jpeg"
		}
	case "video":
		if item.Kind == "video" && item.URL != "" {
			return item.URL, firstString(item.ContentType, "video/mp4")
		}
	}
	return "", ""
}

func safeRemoteURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return false
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return true
	}
	return !ip.IsLoopback() &&
		!ip.IsPrivate() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsUnspecified() &&
		!ip.IsMulticast() &&
		!ip.IsInterfaceLocalMulticast() &&
		!ip.IsLinkLocalMulticast()
}

func sanitizeLogError(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	if len(value) > 180 {
		return value[:180]
	}
	return value
}

func firstString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (h *Handlers) publicURL(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return h.publicBaseURL + path
}
