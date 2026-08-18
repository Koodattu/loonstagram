package instagram

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestFetchPostFallsBackToOriginalPageAfterEmbedParseFailure(t *testing.T) {
	ref := Ref{Type: TypePost, Shortcode: "ABC123xyz"}
	client := NewClient(ClientConfig{Timeout: time.Second})
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `<html><img src="https://scontent.cdninstagram.com/profile.jpg"></html>`
		if !strings.Contains(req.URL.Path, "/embed/") {
			body = `
<meta property="og:url" content="https://www.instagram.com/p/ABC123xyz/">
<meta property="og:description" content="Loonstagram_user on June 1, 2026: &quot;Fallback caption&quot;">
<meta property="og:image" content="https://scontent.cdninstagram.com/post.jpg">
`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})

	post, err := client.FetchPost(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchPost() error = %v", err)
	}
	if post.Username != "Loonstagram_user" {
		t.Fatalf("Username = %q", post.Username)
	}
	if len(post.Media) != 1 || post.Media[0].URL != "https://scontent.cdninstagram.com/post.jpg" {
		t.Fatalf("Media = %#v", post.Media)
	}
}

func TestFetchPostFallsBackToOriginalPageAfterBlockedEmbed(t *testing.T) {
	ref := Ref{Type: TypePost, Shortcode: "ABC123xyz"}
	client := NewClient(ClientConfig{Timeout: time.Second})
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		status := http.StatusTooManyRequests
		body := "blocked"
		if !strings.Contains(req.URL.Path, "/embed/") {
			status = http.StatusOK
			body = `
<meta property="og:url" content="https://www.instagram.com/p/ABC123xyz/">
<meta property="og:title" content="@loonletwow on Instagram">
<meta property="og:image" content="https://scontent.cdninstagram.com/post.jpg">
`
		}
		return &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})

	post, err := client.FetchPost(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchPost() error = %v", err)
	}
	if post.Username != "loonletwow" || len(post.Media) != 1 {
		t.Fatalf("post = %#v", post)
	}
}

func TestFetchPostRejectsRedirectAwayFromRequestedPost(t *testing.T) {
	ref := Ref{Type: TypePost, Shortcode: "ABC123xyz"}
	client := NewClient(ClientConfig{Timeout: time.Second})
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		redirected := req.Clone(req.Context())
		redirected.URL, _ = req.URL.Parse("https://www.instagram.com/accounts/login/")
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`<meta property="og:title" content="Instagram">`)),
			Request:    redirected,
		}, nil
	})

	_, err := client.FetchPost(context.Background(), ref)
	if err == nil {
		t.Fatal("FetchPost() succeeded after login redirect")
	}
	var fetchErr FetchError
	if !errors.As(err, &fetchErr) || fetchErr.Kind != FetchErrorBlocked {
		t.Fatalf("FetchPost() error = %#v, want blocked", err)
	}
}

func TestFetchPostAllowsSameShortcodeCanonicalTypeRedirect(t *testing.T) {
	ref := Ref{Type: TypeTV, Shortcode: "ABC123xyz"}
	client := NewClient(ClientConfig{Timeout: time.Second})
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		redirected := req.Clone(req.Context())
		redirected.URL, _ = req.URL.Parse("https://www.instagram.com/reel/ABC123xyz/")
		body := `
<meta property="og:url" content="https://www.instagram.com/reel/ABC123xyz/">
<meta property="og:title" content="@loonletwow on Instagram">
<meta property="og:image" content="https://scontent.cdninstagram.com/post.jpg">
`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    redirected,
		}, nil
	})

	post, err := client.FetchPost(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchPost() error = %v", err)
	}
	if post.Username != "loonletwow" || len(post.Media) != 1 {
		t.Fatalf("post = %#v", post)
	}
}

func TestFetchPostFallsBackToOriginalPageAfterCroppedEmbedMedia(t *testing.T) {
	ref := Ref{Type: TypePost, Shortcode: "ABC123xyz"}
	client := NewClient(ClientConfig{Timeout: time.Second})
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `
<script>
  window.__data = {"items":[{
    "code":"ABC123xyz",
    "user":{"username":"loonletwow"},
    "caption":{"text":"caption"},
    "media_type":1,
    "image_versions2":{"candidates":[
      {"url":"https://scontent.cdninstagram.com/cropped.jpg?stp=c288.0.864.864a_dst-jpg_e35_s640x640_tt6","width":864,"height":864}
    ]}
  }]};
</script>`
		if !strings.Contains(req.URL.Path, "/embed/") {
			body = `
<script>
  window.__data = {"items":[{
    "code":"ABC123xyz",
    "user":{"username":"loonletwow"},
    "caption":{"text":"caption"},
    "media_type":1,
    "image_versions2":{"candidates":[
      {"url":"https://scontent.cdninstagram.com/full.jpg?stp=dst-jpg_e35_tt6","width":657,"height":657}
    ]}
  }]};
</script>`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})

	post, err := client.FetchPost(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchPost() error = %v", err)
	}
	if len(post.Media) != 1 || post.Media[0].URL != "https://scontent.cdninstagram.com/full.jpg?stp=dst-jpg_e35_tt6" {
		t.Fatalf("Media = %#v", post.Media)
	}
}

func TestFetchPostFallsBackToOriginalPageForPosterOnlyVideo(t *testing.T) {
	ref := Ref{Type: TypeReel, Shortcode: "ABC123xyz"}
	client := NewClient(ClientConfig{Timeout: time.Second})
	requests := 0
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		body := `
<script>
  window.__data = {"shortcode_media":{
    "shortcode":"ABC123xyz",
    "owner":{"username":"loonletwow"},
    "edge_media_to_caption":{"edges":[{"node":{"text":"Fresh reel"}}]},
    "is_video":true,
    "display_url":"https://scontent.cdninstagram.com/poster.jpg"
  }};
</script>`
		if !strings.Contains(req.URL.Path, "/embed/") {
			body = `
<script>
  window.__data = {"payload":{"media":{
    "code":"ABC123xyz",
    "user":{"username":"loonletwow"},
    "caption":{"text":"Fresh reel"},
    "media_type":2,
    "image_versions2":{"candidates":[
      {"url":"https://scontent.cdninstagram.com/poster.jpg","width":720,"height":1280}
    ]},
    "video_versions":[
      {"url":"https://scontent.cdninstagram.com/video.mp4","width":720,"height":1280}
    ]
  }}};
</script>`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})

	post, err := client.FetchPost(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchPost() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	if len(post.Media) != 1 ||
		post.Media[0].Kind != "video" ||
		post.Media[0].URL != "https://scontent.cdninstagram.com/video.mp4" ||
		post.Media[0].PosterURL != "https://scontent.cdninstagram.com/poster.jpg" {
		t.Fatalf("Media = %#v", post.Media)
	}
}
