package instagram

import (
	"context"
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
