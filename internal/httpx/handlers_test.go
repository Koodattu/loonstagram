package httpx

import (
	"bytes"
	"html/template"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"

	"Loonstagram/internal/instagram"
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

func TestFitImageJPEGUsesDiscordPreviewSize(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 300, 900))
	for y := 0; y < 900; y++ {
		for x := 0; x < 300; x++ {
			source.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 180, A: 255})
		}
	}

	body, err := fitImageJPEG(source, discordPreviewWidth, discordPreviewHeight)
	if err != nil {
		t.Fatalf("fitImageJPEG() error = %v", err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("jpeg.Decode() error = %v", err)
	}
	if decoded.Bounds().Dx() != discordPreviewWidth || decoded.Bounds().Dy() != discordPreviewHeight {
		t.Fatalf("decoded size = %dx%d", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}

func TestFitImagesJPEGUsesDiscordPreviewSizeForCarousel(t *testing.T) {
	sources := []image.Image{
		solidImage(200, 200, color.RGBA{R: 255, A: 255}),
		solidImage(200, 400, color.RGBA{G: 255, A: 255}),
		solidImage(400, 200, color.RGBA{B: 255, A: 255}),
		solidImage(300, 300, color.RGBA{R: 255, G: 255, A: 255}),
	}

	body, err := fitImagesJPEG(sources, discordPreviewWidth, discordPreviewHeight)
	if err != nil {
		t.Fatalf("fitImagesJPEG() error = %v", err)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("jpeg.Decode() error = %v", err)
	}
	if decoded.Bounds().Dx() != discordPreviewWidth || decoded.Bounds().Dy() != discordPreviewHeight {
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

func solidImage(width, height int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, c)
		}
	}
	return img
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
