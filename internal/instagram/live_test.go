package instagram

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestLiveFetchPost(t *testing.T) {
	rawURL := os.Getenv("INSTAGRAM_LIVE_TEST_URL")
	if rawURL == "" {
		t.Skip("set INSTAGRAM_LIVE_TEST_URL to run the live Instagram contract test")
	}

	ref, err := NormalizeURL(rawURL)
	if err != nil {
		t.Fatalf("NormalizeURL() error = %v", err)
	}
	client := NewClient(ClientConfig{Timeout: 15 * time.Second, MaxBodyBytes: 4 * 1024 * 1024})
	post, err := client.FetchPost(context.Background(), ref)
	if err != nil {
		t.Fatalf("FetchPost() error = %v", err)
	}
	if post.Username == "" || len(post.Media) == 0 {
		t.Fatalf("FetchPost() returned incomplete post: username = %q, media = %d", post.Username, len(post.Media))
	}
	for i, item := range post.Media {
		if item.URL != "" && !IsInstagramMediaURL(item.URL) {
			t.Fatalf("media %d has unexpected URL host", i+1)
		}
		if item.PosterURL != "" && !IsInstagramMediaURL(item.PosterURL) {
			t.Fatalf("media %d has unexpected poster URL host", i+1)
		}
	}
	t.Logf("fetched @%s with %d media items", post.Username, len(post.Media))
}
