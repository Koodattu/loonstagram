package automation

import (
	"context"
	"testing"
	"time"

	"Loonstagram/internal/cache"
	"Loonstagram/internal/instagram"
)

type fakeProfileFetcher struct {
	media []instagram.RecentMedia
	limit int
	calls int
}

func (f *fakeProfileFetcher) FetchRecentMedia(ctx context.Context, username string, limit int) ([]instagram.RecentMedia, error) {
	f.calls++
	f.limit = limit
	return f.media, nil
}

type fakePostFetcher struct {
	posts map[string]*instagram.Post
	calls []instagram.Ref
}

func (f *fakePostFetcher) FetchPost(ctx context.Context, ref instagram.Ref) (*instagram.Post, error) {
	f.calls = append(f.calls, ref)
	if f.posts != nil {
		if post := f.posts[ref.Shortcode]; post != nil {
			copy := *post
			return &copy, nil
		}
	}
	return &instagram.Post{
		Username: "loonletwow",
		Caption:  "cached " + ref.Shortcode,
		Media: []instagram.MediaItem{{
			Kind: "image",
			URL:  "https://scontent.cdninstagram.com/" + ref.Shortcode + ".jpg",
		}},
	}, nil
}

type fakeDiscordSender struct {
	messages []string
}

func (f *fakeDiscordSender) SendWebhook(ctx context.Context, webhookURL, content string) error {
	f.messages = append(f.messages, content)
	return nil
}

func TestPollerSeedsFirstRunAndPostsNewMedia(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	if err := store.SaveAutomationConfig(ctx, "loonletwow", true, 30, time.Now()); err != nil {
		t.Fatalf("SaveAutomationConfig() error = %v", err)
	}
	if err := store.SetDiscordWebhook(ctx, cache.DiscordWebhook{URL: "https://discord.com/api/webhooks/123/token"}, time.Now()); err != nil {
		t.Fatalf("SetDiscordWebhook() error = %v", err)
	}

	profiles := &fakeProfileFetcher{media: []instagram.RecentMedia{
		recentMedia(instagram.TypePost, "DEF456xyz"),
		recentMedia(instagram.TypePost, "ABC123xyz"),
	}}
	posts := &fakePostFetcher{}
	discord := &fakeDiscordSender{}
	poller := NewPoller(Options{
		Store:         store,
		Profiles:      profiles,
		Posts:         posts,
		Discord:       discord,
		PublicBaseURL: "https://loonstagram.com",
		Interval:      time.Minute,
		CacheTTL:      time.Hour,
	})

	if err := poller.CheckOnce(ctx); err != nil {
		t.Fatalf("first CheckOnce() error = %v", err)
	}
	if profiles.limit != defaultFetchLimit {
		t.Fatalf("profile fetch limit = %d, want %d", profiles.limit, defaultFetchLimit)
	}
	if len(discord.messages) != 0 {
		t.Fatalf("first run sent %d messages", len(discord.messages))
	}
	if len(posts.calls) != 2 {
		t.Fatalf("first run post fetches = %d, want 2", len(posts.calls))
	}
	if got, ok, err := store.Get(ctx, instagram.Ref{Type: instagram.TypePost, Shortcode: "DEF456xyz"}, time.Now()); err != nil {
		t.Fatalf("Get() error = %v", err)
	} else if !ok || len(got.Media) != 1 {
		t.Fatalf("seeded cache post = %#v, ok = %v", got, ok)
	}

	profiles.media = []instagram.RecentMedia{
		recentMedia(instagram.TypePost, "GHI789xyz"),
		recentMedia(instagram.TypePost, "DEF456xyz"),
		recentMedia(instagram.TypePost, "ABC123xyz"),
	}
	if err := poller.CheckOnce(ctx); err != nil {
		t.Fatalf("second CheckOnce() error = %v", err)
	}
	if len(discord.messages) != 1 {
		t.Fatalf("second run sent %d messages", len(discord.messages))
	}
	if discord.messages[0] != "https://loonstagram.com/p/GHI789xyz" {
		t.Fatalf("message = %q", discord.messages[0])
	}
	if len(posts.calls) != 3 {
		t.Fatalf("total post fetches = %d, want 3", len(posts.calls))
	}
	if got, ok, err := store.Get(ctx, instagram.Ref{Type: instagram.TypePost, Shortcode: "GHI789xyz"}, time.Now()); err != nil {
		t.Fatalf("Get() error = %v", err)
	} else if !ok || len(got.Media) != 1 {
		t.Fatalf("new cache post = %#v, ok = %v", got, ok)
	}
}

func TestPollerDoesNotRefetchCachedInitialPosts(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	now := time.Now()
	ref := instagram.Ref{Type: instagram.TypePost, Shortcode: "ABC123xyz"}
	if err := store.Put(ctx, &instagram.Post{
		Ref:       ref,
		Username:  "loonletwow",
		Media:     []instagram.MediaItem{{Kind: "image", URL: "https://scontent.cdninstagram.com/cached.jpg"}},
		Status:    "ok",
		FetchedAt: now,
		ExpiresAt: now.Add(-time.Hour),
	}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := store.SaveAutomationConfig(ctx, "loonletwow", true, 30, now); err != nil {
		t.Fatalf("SaveAutomationConfig() error = %v", err)
	}
	if err := store.SetDiscordWebhook(ctx, cache.DiscordWebhook{URL: "https://discord.com/api/webhooks/123/token"}, now); err != nil {
		t.Fatalf("SetDiscordWebhook() error = %v", err)
	}

	posts := &fakePostFetcher{}
	poller := NewPoller(Options{
		Store:         store,
		Profiles:      &fakeProfileFetcher{media: []instagram.RecentMedia{recentMedia(instagram.TypePost, "ABC123xyz")}},
		Posts:         posts,
		Discord:       &fakeDiscordSender{},
		PublicBaseURL: "https://loonstagram.com",
		Interval:      time.Minute,
		CacheTTL:      time.Hour,
	})

	if err := poller.CheckOnce(ctx); err != nil {
		t.Fatalf("CheckOnce() error = %v", err)
	}
	if len(posts.calls) != 0 {
		t.Fatalf("post fetches = %d, want 0", len(posts.calls))
	}
}

func TestPollerCachesWithoutDiscordAndDoesNotBackfillLater(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	now := time.Now()
	if err := store.SaveAutomationConfig(ctx, "loonletwow", true, 30, now); err != nil {
		t.Fatalf("SaveAutomationConfig() error = %v", err)
	}

	profiles := &fakeProfileFetcher{media: []instagram.RecentMedia{recentMedia(instagram.TypePost, "ABC123xyz")}}
	posts := &fakePostFetcher{}
	discord := &fakeDiscordSender{}
	poller := NewPoller(Options{
		Store:         store,
		Profiles:      profiles,
		Posts:         posts,
		Discord:       discord,
		PublicBaseURL: "https://loonstagram.com",
		Interval:      time.Minute,
		CacheTTL:      time.Hour,
	})

	if err := poller.CheckOnce(ctx); err != nil {
		t.Fatalf("first CheckOnce() error = %v", err)
	}
	if len(posts.calls) != 1 {
		t.Fatalf("post fetches = %d, want 1", len(posts.calls))
	}
	if len(discord.messages) != 0 {
		t.Fatalf("messages without discord = %d, want 0", len(discord.messages))
	}

	if err := store.SetDiscordWebhook(ctx, cache.DiscordWebhook{URL: "https://discord.com/api/webhooks/123/token"}, now); err != nil {
		t.Fatalf("SetDiscordWebhook() error = %v", err)
	}
	if err := poller.CheckOnce(ctx); err != nil {
		t.Fatalf("second CheckOnce() error = %v", err)
	}
	if len(discord.messages) != 0 {
		t.Fatalf("backfilled messages = %d, want 0", len(discord.messages))
	}
}

func TestPollerSkipsProfileFetchDuringBlockedCooldown(t *testing.T) {
	ctx := context.Background()
	store, err := cache.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("cache.Open() error = %v", err)
	}
	defer store.Close()

	now := time.Now()
	if err := store.SaveAutomationConfig(ctx, "loonletwow", true, 30, now); err != nil {
		t.Fatalf("SaveAutomationConfig() error = %v", err)
	}
	if err := store.UpdateAutomationRun(ctx, now, time.Time{}, "Instagram profile refresh failed.", "instagram profile fetch blocked"); err != nil {
		t.Fatalf("UpdateAutomationRun() error = %v", err)
	}

	profiles := &fakeProfileFetcher{media: []instagram.RecentMedia{recentMedia(instagram.TypePost, "ABC123xyz")}}
	poller := NewPoller(Options{
		Store:         store,
		Profiles:      profiles,
		Posts:         &fakePostFetcher{},
		Discord:       &fakeDiscordSender{},
		PublicBaseURL: "https://loonstagram.com",
		Interval:      time.Minute,
		CacheTTL:      time.Hour,
	})

	if err := poller.CheckOnce(ctx); err != nil {
		t.Fatalf("CheckOnce() error = %v", err)
	}
	if profiles.calls != 0 {
		t.Fatalf("profile fetch calls = %d, want 0", profiles.calls)
	}
}

func recentMedia(mediaType, shortcode string) instagram.RecentMedia {
	ref, err := instagram.NewRef(mediaType, shortcode)
	if err != nil {
		panic(err)
	}
	return instagram.RecentMedia{
		Ref:          ref,
		Username:     "loonletwow",
		InstagramURL: ref.OriginalURL(),
		TakenAt:      time.Unix(1710000000, 0),
	}
}
