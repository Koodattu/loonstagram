package cache

import (
	"context"
	"testing"
	"time"

	"instafix/internal/instagram"
)

func TestCacheHitAndExpiry(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Unix(1000, 0)
	ref := instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"}
	post := &instagram.Post{
		Ref:         ref,
		OriginalURL: ref.OriginalURL(),
		Username:    "instafix_user",
		Caption:     "caption",
		Media: []instagram.MediaItem{{
			Kind:        "image",
			URL:         "https://scontent.cdninstagram.com/image.jpg",
			ContentType: "image/jpeg",
		}},
		Status:    "ok",
		FetchedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}

	if err := store.Put(ctx, post); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	got, ok, err := store.Get(ctx, ref, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if got.Username != "instafix_user" || len(got.Media) != 1 {
		t.Fatalf("cached post = %#v", got)
	}

	_, ok, err = store.Get(ctx, ref, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("Get expired() error = %v", err)
	}
	if ok {
		t.Fatal("expired row should be ignored")
	}
}
