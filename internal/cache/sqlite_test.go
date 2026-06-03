package cache

import (
	"context"
	"testing"
	"time"

	"Loonstagram/internal/instagram"
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
		Username:    "Loonstagram_user",
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
	if got.Username != "Loonstagram_user" || len(got.Media) != 1 {
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

func TestDeleteRemovesSinglePost(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Unix(1000, 0)
	ref := instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"}
	if err := store.Put(ctx, &instagram.Post{
		Ref:       ref,
		Status:    "ok",
		FetchedAt: now,
		ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	count, err := store.Delete(ctx, ref)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("Delete() count = %d, want 1", count)
	}
	if _, ok, err := store.Get(ctx, ref, now); err != nil {
		t.Fatalf("Get() error = %v", err)
	} else if ok {
		t.Fatal("deleted row should not be returned")
	}
}
