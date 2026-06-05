package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"Loonstagram/internal/cache"
	"Loonstagram/internal/instagram"
	"Loonstagram/internal/mediacache"
	"Loonstagram/web"
)

type PostFetcher interface {
	FetchPost(ctx context.Context, ref instagram.Ref) (*instagram.Post, error)
}

type PostDebugger interface {
	DebugFetchPost(ctx context.Context, ref instagram.Ref) instagram.DebugReport
}

type Options struct {
	PublicBaseURL       string
	CacheSuccessTTL     time.Duration
	CacheNegativeTTL    time.Duration
	CacheBlockedTTL     time.Duration
	MediaProxyMode      string
	AdminToken          string
	DiscordClientID     string
	DiscordClientSecret string
	DiscordRedirectURL  string
	Store               *cache.Store
	MediaCache          *mediacache.Store
	Scraper             PostFetcher
	Logger              *slog.Logger
}

type Handlers struct {
	publicBaseURL       string
	cacheSuccessTTL     time.Duration
	cacheNegativeTTL    time.Duration
	cacheBlockedTTL     time.Duration
	mediaProxyMode      string
	adminToken          string
	discordClientID     string
	discordClientSecret string
	discordRedirectURL  string
	store               *cache.Store
	mediaCache          *mediacache.Store
	scraper             PostFetcher
	logger              *slog.Logger
	templates           *template.Template
	flight              *flight
	mediaFlight         *mediaFlight
	mediaClient         *http.Client
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
		publicBaseURL:       strings.TrimRight(opts.PublicBaseURL, "/"),
		cacheSuccessTTL:     opts.CacheSuccessTTL,
		cacheNegativeTTL:    opts.CacheNegativeTTL,
		cacheBlockedTTL:     opts.CacheBlockedTTL,
		mediaProxyMode:      opts.MediaProxyMode,
		adminToken:          opts.AdminToken,
		discordClientID:     opts.DiscordClientID,
		discordClientSecret: opts.DiscordClientSecret,
		discordRedirectURL:  strings.TrimSpace(opts.DiscordRedirectURL),
		store:               opts.Store,
		mediaCache:          opts.MediaCache,
		scraper:             opts.Scraper,
		logger:              opts.Logger,
		templates:           templates,
		flight:              newFlight(),
		mediaFlight:         newMediaFlight(),
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
	mux.HandleFunc("GET /admin", h.admin)
	mux.HandleFunc("GET /api/gallery", h.gallery)
	mux.HandleFunc("POST /api/convert", h.convert)
	mux.HandleFunc("GET /p/{shortcode}", h.canonical(instagram.TypePost))
	mux.HandleFunc("GET /reel/{shortcode}", h.canonical(instagram.TypeReel))
	mux.HandleFunc("GET /tv/{shortcode}", h.canonical(instagram.TypeTV))
	mux.HandleFunc("GET /media/{type}/{shortcode}/{index}/image", h.media("image"))
	mux.HandleFunc("GET /media/{type}/{shortcode}/{index}/video", h.media("video"))
	mux.HandleFunc("GET /preview/{type}/{shortcode}/image", h.previewImage)
	mux.HandleFunc("GET /api/automation/status", h.automationStatus)
	mux.HandleFunc("POST /api/automation/config", h.saveAutomationConfig)
	mux.HandleFunc("POST /api/automation/discord/webhook", h.saveDiscordWebhook)
	mux.HandleFunc("POST /api/automation/discord/disconnect", h.disconnectDiscordWebhook)
	mux.HandleFunc("POST /api/automation/test", h.testDiscordWebhook)
	mux.HandleFunc("GET /oauth/discord/start", h.startDiscordOAuth)
	mux.HandleFunc("GET /oauth/discord/callback", h.discordOAuthCallback)
	mux.HandleFunc("GET /debug", h.debugFromQuery)
	mux.HandleFunc("GET /debug/{type}/{shortcode}", h.debugCanonical)
	mux.HandleFunc("POST /debug/{type}/{shortcode}/refresh", h.refreshDebugCache)
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

func (h *Handlers) admin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "admin.html", map[string]string{
		"PublicBaseURL": h.publicBaseURL,
	}); err != nil {
		h.logger.Error("render admin", "error", err)
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

const defaultGalleryUsername = "loonletwow"

type galleryResponse struct {
	OK       bool          `json:"ok"`
	Profile  string        `json:"profile"`
	Source   string        `json:"source"`
	Items    []galleryItem `json:"items"`
	Error    string        `json:"error,omitempty"`
	Empty    string        `json:"empty,omitempty"`
	Updated  string        `json:"updated,omitempty"`
}

type galleryItem struct {
	Type         string         `json:"type"`
	Shortcode    string         `json:"shortcode"`
	Username     string         `json:"username"`
	Caption      string         `json:"caption"`
	OriginalURL  string         `json:"originalUrl"`
	CanonicalURL string         `json:"canonicalUrl"`
	FetchedAt    string         `json:"fetchedAt,omitempty"`
	Media        []galleryMedia `json:"media"`
}

type galleryMedia struct {
	Kind     string `json:"kind"`
	ImageURL string `json:"imageUrl,omitempty"`
	VideoURL string `json:"videoUrl,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

func (h *Handlers) gallery(w http.ResponseWriter, r *http.Request) {
	username := defaultGalleryUsername
	source := "default"
	settings, err := h.store.GetAutomationSettings(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, galleryResponse{OK: false, Error: "Could not load gallery"})
		return
	}
	if settings.InstagramUsername != "" {
		username = settings.InstagramUsername
		source = "automation"
	}

	posts, err := h.store.ListGalleryPosts(r.Context(), username, 120, time.Now())
	if err != nil {
		h.logger.Error("load gallery", "error", err)
		writeJSON(w, http.StatusInternalServerError, galleryResponse{OK: false, Error: "Could not load gallery"})
		return
	}

	items := make([]galleryItem, 0, len(posts))
	for i := range posts {
		item := h.galleryItem(&posts[i])
		if len(item.Media) == 0 {
			continue
		}
		items = append(items, item)
	}
	empty := ""
	if len(items) == 0 {
		empty = "No cached gallery posts yet. Create a fixed URL and let Discord preview it for this profile."
	} else {
		h.warmGalleryMedia(posts)
	}
	writeJSON(w, http.StatusOK, galleryResponse{
		OK:      true,
		Profile: username,
		Source:  source,
		Items:   items,
		Empty:   empty,
		Updated: formatTime(time.Now()),
	})
}

func (h *Handlers) galleryItem(post *instagram.Post) galleryItem {
	item := galleryItem{
		Type:         post.Ref.Type,
		Shortcode:    post.Ref.Shortcode,
		Username:     post.Username,
		Caption:      instagram.CleanCaption(post.Caption),
		OriginalURL:  firstString(post.OriginalURL, post.Ref.OriginalURL()),
		CanonicalURL: h.publicURL(post.Ref.CanonicalPath()),
		FetchedAt:    formatTime(post.FetchedAt),
	}
	for i, media := range post.Media {
		imageURL, _ := mediaTarget("image", media)
		videoURL, _ := mediaTarget("video", media)
		if imageURL == "" && videoURL == "" {
			continue
		}
		galleryMedia := galleryMedia{
			Kind:   media.Kind,
			Width:  media.Width,
			Height: media.Height,
		}
		if imageURL != "" {
			galleryMedia.ImageURL = h.publicURL(fmt.Sprintf("/media/%s/%s/%d/image", post.Ref.Type, post.Ref.Shortcode, i+1))
		}
		if videoURL != "" {
			galleryMedia.VideoURL = h.publicURL(fmt.Sprintf("/media/%s/%s/%d/video", post.Ref.Type, post.Ref.Shortcode, i+1))
		}
		item.Media = append(item.Media, galleryMedia)
	}
	return item
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

type mediaWarmTarget struct {
	Key         string
	Target      string
	ContentType string
}

func (h *Handlers) warmGalleryMedia(posts []instagram.Post) {
	if h.mediaCache == nil {
		return
	}
	targets := make([]mediaWarmTarget, 0, len(posts))
	seen := make(map[string]bool)
	for i := range posts {
		post := &posts[i]
		if post.Status != "ok" {
			continue
		}
		for mediaIndex, media := range post.Media {
			target, contentType := mediaTarget("image", media)
			if target == "" || !safeRemoteURL(target) {
				continue
			}
			key := mediaCacheKey(post.Ref, mediaIndex+1, "image")
			if seen[key] {
				continue
			}
			seen[key] = true
			targets = append(targets, mediaWarmTarget{
				Key:         key,
				Target:      target,
				ContentType: contentType,
			})
		}
	}
	if len(targets) == 0 {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		const workers = 4
		jobs := make(chan mediaWarmTarget)
		done := make(chan struct{}, workers)
		for i := 0; i < workers; i++ {
			go func() {
				defer func() { done <- struct{}{} }()
				for target := range jobs {
					if err := h.cacheRemoteMedia(ctx, target.Key, target.Target, target.ContentType); err != nil {
						h.logger.Debug("gallery media warm failed", "key", target.Key, "error", sanitizeLogError(err))
					}
				}
			}()
		}
		for _, target := range targets {
			select {
			case <-ctx.Done():
				close(jobs)
				for i := 0; i < workers; i++ {
					<-done
				}
				return
			case jobs <- target:
			}
		}
		close(jobs)
		for i := 0; i < workers; i++ {
			<-done
		}
	}()
}

func (h *Handlers) debugFromQuery(w http.ResponseWriter, r *http.Request) {
	ref, err := instagram.NormalizeURL(r.URL.Query().Get("url"))
	if err != nil {
		http.Error(w, "Unsupported Instagram URL", http.StatusBadRequest)
		return
	}
	h.renderDebug(w, r, ref)
}

func (h *Handlers) debugCanonical(w http.ResponseWriter, r *http.Request) {
	ref, err := instagram.NewRef(r.PathValue("type"), r.PathValue("shortcode"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.renderDebug(w, r, ref)
}

type debugPageData struct {
	Title       string
	Ref         instagram.Ref
	OriginalURL string
	EmbedURL    string
	Cache       debugCacheData
	Action      debugActionData
	Fresh       instagram.DebugReport
	ParsedPost  *instagram.Post
	Embed       embedData
	Media       []debugMediaData
	DumpJSON    string
}

type debugActionData struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type debugCacheData struct {
	Checked bool            `json:"checked"`
	Hit     bool            `json:"hit"`
	Error   string          `json:"error,omitempty"`
	Post    *instagram.Post `json:"post,omitempty"`
}

type debugMediaData struct {
	Index           int    `json:"index"`
	Kind            string `json:"kind"`
	ImageURL        string `json:"imageUrl,omitempty"`
	ImagePreviewURL string `json:"imagePreviewUrl,omitempty"`
	VideoURL        string `json:"videoUrl,omitempty"`
	VideoPreviewURL string `json:"videoPreviewUrl,omitempty"`
	PosterURL       string `json:"posterUrl,omitempty"`
	Width           int    `json:"width,omitempty"`
	Height          int    `json:"height,omitempty"`
	RemoteURL       string `json:"remoteUrl,omitempty"`
	PublicURL       string `json:"publicUrl,omitempty"`
	ContentType     string `json:"contentType,omitempty"`
}

func (h *Handlers) renderDebug(w http.ResponseWriter, r *http.Request, ref instagram.Ref) {
	debugger, ok := h.scraper.(PostDebugger)
	if !ok {
		http.Error(w, "Debug fetch is not supported by this scraper", http.StatusNotImplemented)
		return
	}

	setCacheStatus(r.Context(), "debug")
	cacheData := h.debugCache(r.Context(), ref)
	fresh := debugger.DebugFetchPost(r.Context(), ref)
	parsedPost := bestDebugPost(fresh, cacheData.Post)

	data := debugPageData{
		Title:       "Loonstagram debug",
		Ref:         ref,
		OriginalURL: ref.OriginalURL(),
		EmbedURL:    ref.EmbedURL(),
		Cache:       cacheData,
		Action: debugActionData{
			Message: strings.TrimSpace(r.URL.Query().Get("message")),
			Error:   strings.TrimSpace(r.URL.Query().Get("error")),
		},
		Fresh:      fresh,
		ParsedPost: parsedPost,
	}
	if parsedPost != nil {
		data.Embed = h.embedData(parsedPost)
		data.Media = h.debugMedia(parsedPost)
	}

	dump := struct {
		Ref         instagram.Ref         `json:"ref"`
		OriginalURL string                `json:"originalUrl"`
		EmbedURL    string                `json:"embedUrl"`
		Cache       debugCacheData        `json:"cache"`
		Action      debugActionData       `json:"action,omitempty"`
		Fresh       instagram.DebugReport `json:"fresh"`
		Embed       embedData             `json:"embedData"`
		Media       []debugMediaData      `json:"media"`
		ParsedPost  *instagram.Post       `json:"parsedPost,omitempty"`
	}{
		Ref:         ref,
		OriginalURL: data.OriginalURL,
		EmbedURL:    data.EmbedURL,
		Cache:       data.Cache,
		Action:      data.Action,
		Fresh:       fresh,
		Embed:       data.Embed,
		Media:       data.Media,
		ParsedPost:  parsedPost,
	}
	if dumpJSON, err := json.MarshalIndent(dump, "", "  "); err == nil {
		data.DumpJSON = string(dumpJSON)
	} else {
		data.DumpJSON = err.Error()
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "debug.html", data); err != nil {
		h.logger.Error("render debug", "shortcode", ref.Shortcode, "media_type", ref.Type, "error", err)
	}
}

func (h *Handlers) refreshDebugCache(w http.ResponseWriter, r *http.Request) {
	ref, err := instagram.NewRef(r.PathValue("type"), r.PathValue("shortcode"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if _, err := h.store.Delete(r.Context(), ref); err != nil {
		h.redirectDebug(w, r, ref, "", "Could not clear cache: "+sanitizeLogError(err))
		return
	}

	post, err := h.getOrFetchPost(r.Context(), ref)
	if err != nil {
		h.redirectDebug(w, r, ref, "", "Could not refresh cache: "+sanitizeLogError(err))
		return
	}
	if post == nil {
		h.redirectDebug(w, r, ref, "", "Could not refresh cache: no post returned")
		return
	}
	if post.Status != "ok" {
		h.redirectDebug(w, r, ref, "Cache refreshed, but provider status is "+post.Status+".", "")
		return
	}

	h.redirectDebug(w, r, ref, "Cache refreshed. Discord will see a new preview image URL on the next crawl.", "")
}

func (h *Handlers) redirectDebug(w http.ResponseWriter, r *http.Request, ref instagram.Ref, message, errText string) {
	values := url.Values{}
	if message != "" {
		values.Set("message", message)
	}
	if errText != "" {
		values.Set("error", errText)
	}

	target := fmt.Sprintf("/debug/%s/%s", ref.Type, ref.Shortcode)
	if encoded := values.Encode(); encoded != "" {
		target += "?" + encoded
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (h *Handlers) debugCache(ctx context.Context, ref instagram.Ref) debugCacheData {
	out := debugCacheData{Checked: true}
	post, ok, err := h.store.Get(ctx, ref, time.Now())
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.Hit = ok
	if ok {
		out.Post = post
	}
	return out
}

func (h *Handlers) debugMedia(post *instagram.Post) []debugMediaData {
	out := make([]debugMediaData, 0, len(post.Media))
	for i, item := range post.Media {
		imageURL, imageType := mediaTarget("image", item)
		videoURL, videoType := mediaTarget("video", item)
		remoteURL := firstString(imageURL, videoURL)
		contentType := firstString(imageType, videoType, item.ContentType)
		publicURL := ""
		imagePreviewURL := ""
		if imageURL != "" {
			publicURL = h.publicURL(fmt.Sprintf("/media/%s/%s/%d/image", post.Ref.Type, post.Ref.Shortcode, i+1))
			imagePreviewURL = publicURL + "?stream=1"
		}
		videoPreviewURL := ""
		if videoURL != "" {
			videoPreviewURL = h.publicURL(fmt.Sprintf("/media/%s/%s/%d/video", post.Ref.Type, post.Ref.Shortcode, i+1)) + "?stream=1"
		}
		out = append(out, debugMediaData{
			Index:           i + 1,
			Kind:            item.Kind,
			ImageURL:        imageURL,
			ImagePreviewURL: imagePreviewURL,
			VideoURL:        videoURL,
			VideoPreviewURL: videoPreviewURL,
			PosterURL:       item.PosterURL,
			Width:           item.Width,
			Height:          item.Height,
			RemoteURL:       remoteURL,
			PublicURL:       publicURL,
			ContentType:     contentType,
		})
	}
	return out
}

func bestDebugPost(report instagram.DebugReport, cached *instagram.Post) *instagram.Post {
	best := cached
	bestScore := debugPostScore(cached)
	for _, fetch := range report.Fetches {
		if score := debugPostScore(fetch.ParsedPost); score > bestScore {
			best = fetch.ParsedPost
			bestScore = score
		}
	}
	return best
}

func debugPostScore(post *instagram.Post) int {
	if post == nil || (post.Username == "" && post.Caption == "") {
		return 0
	}
	score := 0
	if post.Status == "ok" {
		score++
	}
	if post.Username != "" {
		score += 4
	}
	if post.Caption != "" {
		score += 3
	}
	if len(post.Media) > 0 {
		score += 2
	}
	return score
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
	Images         []embedImage
	VideoURL       string
	HasImage       bool
	HasVideo       bool
	VideoType      string
	ThemeColor     string
	Username       string
	Shortcode      string
	MediaType      string
	ProviderStatus string
}

type embedImage struct {
	URL    string
	Width  int
	Height int
}

const maxEmbedImages = 4

const (
	discordPreviewMaxSize   = 900
	discordPreviewMinAspect = 0.75
	discordPreviewMaxAspect = 1.91
	discordPreviewLimit     = 4
)

func (h *Handlers) embedData(post *instagram.Post) embedData {
	title := "Instagram post"
	if post.Username != "" {
		title = "@" + post.Username
	}

	description := instagram.CleanCaption(post.Caption)
	if description == "" {
		description = "Open this Instagram post."
	}

	data := embedData{
		SiteName:       "Loonstagram",
		Title:          title,
		Description:    description,
		OriginalURL:    post.Ref.OriginalURL(),
		CanonicalURL:   h.publicURL(post.Ref.CanonicalPath()),
		ThemeColor:     "#d62976",
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

	images := previewImages(post.Media)
	for _, image := range images {
		data.Images = append(data.Images, embedImage{
			URL:    h.publicURL(fmt.Sprintf("/media/%s/%s/%d/image", post.Ref.Type, post.Ref.Shortcode, image.index)),
			Width:  image.item.Width,
			Height: image.item.Height,
		})
		if len(data.Images) >= maxEmbedImages {
			break
		}
	}
	if len(data.Images) > 0 {
		data.ImageURL = h.previewImageURL(post)
		data.HasImage = true
	}

	first := firstPreviewMedia(post.Media)
	if first == nil {
		return data
	}

	if first.item.Kind == "video" && first.item.URL != "" {
		data.VideoURL = h.publicURL(fmt.Sprintf("/media/%s/%s/%d/video", post.Ref.Type, post.Ref.Shortcode, first.index))
		data.VideoType = firstString(first.item.ContentType, "video/mp4")
		data.HasVideo = true
	}

	return data
}

func (h *Handlers) previewImageURL(post *instagram.Post) string {
	path := fmt.Sprintf("/preview/%s/%s/image", post.Ref.Type, post.Ref.Shortcode)
	if post.FetchedAt.IsZero() {
		return h.publicURL(path)
	}
	return h.publicURL(path + "?v=" + strconv.FormatInt(post.FetchedAt.Unix(), 10))
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

		cacheKey := mediaCacheKey(ref, index, kind)
		w.Header().Set("Cache-Control", "public, max-age=300")
		if h.mediaCache != nil {
			if served, err := h.serveCachedMedia(w, r, cacheKey); err != nil {
				h.logger.Warn("media cache read failed", "key", cacheKey, "error", sanitizeLogError(err))
			} else if served {
				return
			}
			if err := h.cacheRemoteMedia(r.Context(), cacheKey, target, contentType); err != nil {
				h.logger.Warn("media cache fill failed", "key", cacheKey, "shortcode", ref.Shortcode, "media_type", ref.Type, "kind", kind, "error", sanitizeLogError(err))
				http.Error(w, "Media fetch failed", http.StatusBadGateway)
				return
			}
			if served, err := h.serveCachedMedia(w, r, cacheKey); err != nil {
				h.logger.Warn("media cache serve failed", "key", cacheKey, "error", sanitizeLogError(err))
			} else if served {
				return
			}
			http.Error(w, "Media cache failed", http.StatusBadGateway)
			return
		}

		if h.mediaProxyMode == "redirect" && r.URL.Query().Get("stream") != "1" {
			http.Redirect(w, r, target, http.StatusFound)
			return
		}

		h.streamMedia(w, r, target, contentType)
	}
}

func (h *Handlers) serveCachedMedia(w http.ResponseWriter, r *http.Request, key string) (bool, error) {
	file, entry, ok, err := h.mediaCache.Open(key)
	if err != nil || !ok {
		return false, err
	}
	defer file.Close()

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if entry.ContentType != "" {
		w.Header().Set("Content-Type", entry.ContentType)
	}
	http.ServeContent(w, r, key, entry.CreatedAt, file)
	return true, nil
}

func (h *Handlers) cacheRemoteMedia(ctx context.Context, key, target, fallbackContentType string) error {
	return h.mediaFlight.Do(key, func() error {
		if file, _, ok, err := h.mediaCache.Open(key); err != nil {
			return err
		} else if ok {
			_ = file.Close()
			return nil
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		if err != nil {
			return fmt.Errorf("create media cache request: %w", err)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Loonstagram/1.0)")
		req.Header.Set("Accept", "*/*")

		resp, err := h.mediaClient.Do(req)
		if err != nil {
			return fmt.Errorf("fetch upstream media: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("upstream media returned status %d", resp.StatusCode)
		}
		contentType := firstString(resp.Header.Get("Content-Type"), fallbackContentType)
		if _, err := h.mediaCache.Put(ctx, key, contentType, resp.Body); err != nil {
			return err
		}
		return nil
	})
}

func (h *Handlers) previewImage(w http.ResponseWriter, r *http.Request) {
	ref, err := instagram.NewRef(r.PathValue("type"), r.PathValue("shortcode"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	post, err := h.getOrFetchPost(r.Context(), ref)
	if err != nil || post.Status != "ok" {
		http.NotFound(w, r)
		return
	}

	targets := previewImageTargets(post.Media, discordPreviewLimit)
	if len(targets) == 0 {
		http.NotFound(w, r)
		return
	}

	sources := make([]image.Image, 0, len(targets))
	for _, target := range targets {
		source, err := h.fetchRemoteImage(r.Context(), target)
		if err != nil {
			continue
		}
		sources = append(sources, source)
	}
	if len(sources) == 0 {
		http.Error(w, "Media fetch failed", http.StatusBadGateway)
		return
	}

	body, err := previewImageJPEG(sources)
	if err != nil {
		http.Error(w, "Media fit failed", http.StatusBadGateway)
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	_, _ = w.Write(body)
}

func (h *Handlers) getOrFetchPost(ctx context.Context, ref instagram.Ref) (*instagram.Post, error) {
	now := time.Now()
	if post, ok, err := h.store.Get(ctx, ref, now); err != nil {
		return post, err
	} else if ok {
		if !shouldRefreshCachedPost(post) {
			setCacheStatus(ctx, "hit")
			return post, nil
		}
		setCacheStatus(ctx, "stale")
	} else {
		setCacheStatus(ctx, "miss")
	}

	key := ref.Type + ":" + ref.Shortcode
	return h.flight.Do(key, func() (*instagram.Post, error) {
		now := time.Now()
		if post, ok, err := h.store.Get(ctx, ref, now); err != nil {
			return post, err
		} else if ok && !shouldRefreshCachedPost(post) {
			setCacheStatus(ctx, "hit")
			return post, nil
		} else if ok {
			setCacheStatus(ctx, "stale")
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

func shouldRefreshCachedPost(post *instagram.Post) bool {
	if post == nil || post.Status != "ok" {
		return false
	}
	return len(post.Media) == 0 || (post.Username == "" && post.Caption == "")
}

func (h *Handlers) streamMedia(w http.ResponseWriter, r *http.Request, target, fallbackContentType string) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, target, nil)
	if err != nil {
		http.Error(w, "Bad upstream media URL", http.StatusBadGateway)
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Loonstagram/1.0)")
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

func (h *Handlers) fetchRemoteImage(ctx context.Context, target string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Loonstagram/1.0)")
	req.Header.Set("Accept", "image/jpeg,image/png,*/*;q=0.8")

	resp, err := h.mediaClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("media fetch returned status %d", resp.StatusCode)
	}

	source, _, err := image.Decode(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return nil, err
	}
	return source, nil
}

func fitImageJPEG(source image.Image, width, height int) ([]byte, error) {
	return fitImagesJPEG([]image.Image{source}, width, height)
}

func previewImageJPEG(sources []image.Image) ([]byte, error) {
	width, height := previewCanvasSize(sources)
	return fitImagesJPEG(sources, width, height)
}

func previewCanvasSize(sources []image.Image) (int, int) {
	if len(sources) != 1 {
		return discordPreviewMaxSize, discordPreviewMaxSize
	}

	bounds := sources[0].Bounds()
	sourceWidth := bounds.Dx()
	sourceHeight := bounds.Dy()
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return discordPreviewMaxSize, discordPreviewMaxSize
	}

	aspect := float64(sourceWidth) / float64(sourceHeight)
	if aspect < discordPreviewMinAspect {
		aspect = discordPreviewMinAspect
	}
	if aspect > discordPreviewMaxAspect {
		aspect = discordPreviewMaxAspect
	}

	if aspect >= 1 {
		width := discordPreviewMaxSize
		height := int(float64(width)/aspect + 0.5)
		if height < 1 {
			height = 1
		}
		return width, height
	}

	height := discordPreviewMaxSize
	width := int(float64(height)*aspect + 0.5)
	if width < 1 {
		width = 1
	}
	return width, height
}

func fitImagesJPEG(sources []image.Image, width, height int) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return nil, errors.New("invalid fitted image size")
	}
	if len(sources) == 0 {
		return nil, errors.New("no images to fit")
	}
	if len(sources) > discordPreviewLimit {
		sources = sources[:discordPreviewLimit]
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{R: 30, G: 31, B: 36, A: 255}}, image.Point{}, draw.Src)
	cells := previewCells(len(sources), canvas.Bounds())
	for i, source := range sources {
		drawImageContained(canvas, source, cells[i])
	}

	var body bytes.Buffer
	if err := jpeg.Encode(&body, canvas, &jpeg.Options{Quality: 88}); err != nil {
		return nil, err
	}
	return body.Bytes(), nil
}

func previewCells(count int, bounds image.Rectangle) []image.Rectangle {
	if count <= 1 {
		return []image.Rectangle{bounds}
	}

	gap := 8
	switch count {
	case 2:
		mid := bounds.Min.Y + (bounds.Dy()-gap)/2
		return []image.Rectangle{
			image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, mid),
			image.Rect(bounds.Min.X, mid+gap, bounds.Max.X, bounds.Max.Y),
		}
	case 3:
		midX := bounds.Min.X + (bounds.Dx()-gap)/2
		midY := bounds.Min.Y + (bounds.Dy()-gap)/2
		return []image.Rectangle{
			image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, midY),
			image.Rect(bounds.Min.X, midY+gap, midX, bounds.Max.Y),
			image.Rect(midX+gap, midY+gap, bounds.Max.X, bounds.Max.Y),
		}
	default:
		midX := bounds.Min.X + (bounds.Dx()-gap)/2
		midY := bounds.Min.Y + (bounds.Dy()-gap)/2
		return []image.Rectangle{
			image.Rect(bounds.Min.X, bounds.Min.Y, midX, midY),
			image.Rect(midX+gap, bounds.Min.Y, bounds.Max.X, midY),
			image.Rect(bounds.Min.X, midY+gap, midX, bounds.Max.Y),
			image.Rect(midX+gap, midY+gap, bounds.Max.X, bounds.Max.Y),
		}
	}
}

func drawImageContained(dst *image.RGBA, source image.Image, target image.Rectangle) {
	target = target.Intersect(dst.Bounds())
	if target.Empty() {
		return
	}
	srcBounds := source.Bounds()
	srcWidth := srcBounds.Dx()
	srcHeight := srcBounds.Dy()
	if srcWidth <= 0 || srcHeight <= 0 {
		return
	}

	dstWidth := target.Dx()
	dstHeight := target.Dy()
	scaleX := float64(dstWidth) / float64(srcWidth)
	scaleY := float64(dstHeight) / float64(srcHeight)
	scale := scaleX
	if scaleY < scale {
		scale = scaleY
	}
	scaledWidth := int(float64(srcWidth)*scale + 0.5)
	scaledHeight := int(float64(srcHeight)*scale + 0.5)
	if scaledWidth < 1 {
		scaledWidth = 1
	}
	if scaledHeight < 1 {
		scaledHeight = 1
	}

	offsetX := target.Min.X + (dstWidth-scaledWidth)/2
	offsetY := target.Min.Y + (dstHeight-scaledHeight)/2
	for y := 0; y < scaledHeight; y++ {
		sourceY := srcBounds.Min.Y + int(float64(y)*float64(srcHeight)/float64(scaledHeight))
		if sourceY >= srcBounds.Max.Y {
			sourceY = srcBounds.Max.Y - 1
		}
		for x := 0; x < scaledWidth; x++ {
			sourceX := srcBounds.Min.X + int(float64(x)*float64(srcWidth)/float64(scaledWidth))
			if sourceX >= srcBounds.Max.X {
				sourceX = srcBounds.Max.X - 1
			}
			dst.Set(offsetX+x, offsetY+y, source.At(sourceX, sourceY))
		}
	}
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

type indexedMedia struct {
	index int
	item  instagram.MediaItem
}

func firstPreviewMedia(media []instagram.MediaItem) *indexedMedia {
	for i := range media {
		if imageCandidate(media[i]) != "" {
			return &indexedMedia{index: i + 1, item: media[i]}
		}
	}
	if len(media) == 0 {
		return nil
	}
	return &indexedMedia{index: 1, item: media[0]}
}

func previewImages(media []instagram.MediaItem) []indexedMedia {
	out := make([]indexedMedia, 0, len(media))
	for i := range media {
		if imageCandidate(media[i]) == "" {
			continue
		}
		out = append(out, indexedMedia{index: i + 1, item: media[i]})
	}
	return out
}

func previewImageTargets(media []instagram.MediaItem, limit int) []string {
	if limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for _, item := range media {
		target, _ := mediaTarget("image", item)
		if target == "" || !safeRemoteURL(target) {
			continue
		}
		out = append(out, target)
		if len(out) >= limit {
			return out
		}
	}
	return out
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

func mediaCacheKey(ref instagram.Ref, index int, kind string) string {
	return fmt.Sprintf("%s_%s_%d_%s", ref.Type, ref.Shortcode, index, kind)
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
