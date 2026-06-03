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
}

func (f *fakeProfileFetcher) FetchRecentMedia(ctx context.Context, username string, limit int) ([]instagram.RecentMedia, error) {
	return f.media, nil
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

	if err := store.SaveAutomationConfig(ctx, "loonletwow", true, time.Now()); err != nil {
		t.Fatalf("SaveAutomationConfig() error = %v", err)
	}
	if err := store.SetDiscordWebhook(ctx, cache.DiscordWebhook{URL: "https://discord.com/api/webhooks/123/token"}, time.Now()); err != nil {
		t.Fatalf("SetDiscordWebhook() error = %v", err)
	}

	profiles := &fakeProfileFetcher{media: []instagram.RecentMedia{
		recentMedia(instagram.TypePost, "DEF456xyz"),
		recentMedia(instagram.TypePost, "ABC123xyz"),
	}}
	discord := &fakeDiscordSender{}
	poller := NewPoller(Options{
		Store:         store,
		Profiles:      profiles,
		Discord:       discord,
		PublicBaseURL: "https://loonstagram.com",
		Interval:      time.Minute,
	})

	if err := poller.CheckOnce(ctx); err != nil {
		t.Fatalf("first CheckOnce() error = %v", err)
	}
	if len(discord.messages) != 0 {
		t.Fatalf("first run sent %d messages", len(discord.messages))
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
